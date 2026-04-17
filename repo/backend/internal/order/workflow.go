// Package order implements the fulfillment workflow: a strict state machine
// over placement → picking → dispatch → delivery → receipt → refund, and
// exception detection for timeouts and out-of-stock conditions.
package order

import (
	"errors"
	"fmt"
	"time"
)

// Status is the current stage of an order.
type Status string

const (
	StatusPlaced    Status = "placed"
	StatusPicking   Status = "picking"
	StatusDispatched Status = "dispatched"
	StatusDelivered Status = "delivered"
	StatusReceived  Status = "received"
	StatusRefunded  Status = "refunded"
	StatusCanceled  Status = "canceled"
)

// AllStatuses lists the statuses exposed to the UI for filter dropdowns.
var AllStatuses = []string{
	string(StatusPlaced), string(StatusPicking), string(StatusDispatched),
	string(StatusDelivered), string(StatusReceived), string(StatusRefunded),
	string(StatusCanceled),
}

// PickingTimeout is the window after which an order stuck in picking is
// flagged to the exception queue.
const PickingTimeout = 30 * time.Minute

// transitions documents the allowed forward moves. Terminal statuses
// (received, refunded, canceled) have no outbound transitions.
var transitions = map[Status]map[Status]struct{}{
	StatusPlaced:     {StatusPicking: {}, StatusCanceled: {}},
	StatusPicking:    {StatusDispatched: {}, StatusCanceled: {}, StatusRefunded: {}},
	StatusDispatched: {StatusDelivered: {}, StatusCanceled: {}},
	StatusDelivered:  {StatusReceived: {}, StatusRefunded: {}},
	StatusReceived:   {StatusRefunded: {}},
}

// Errors surfaced by the state machine.
var (
	ErrBadTransition = errors.New("transition not allowed")
	ErrNoReason      = errors.New("refund requires a reason note")
)

// Event is a single entry in the order history. The event stream is
// append-only so state at any point in time can be reconstructed.
type Event struct {
	ID        string
	OrderID   string
	At        time.Time
	From      Status
	To        Status
	Actor     string // user ID
	Reason    string // optional, required for refund
	Note      string // free-form
}

// LineItem represents a single SKU on an order. Availability is tracked
// against the current local inventory; Backordered means pickers couldn't
// fulfill the item from stock. The automatic OOS detector reads this
// field to decide whether to enqueue an exception.
type LineItem struct {
	SKU          string
	Description  string
	Qty          int
	Backordered  bool
}

// Order is the aggregate that the state machine operates on.
type Order struct {
	ID         string
	Status     Status
	CustomerID string
	PlacedAt   time.Time
	UpdatedAt  time.Time
	TotalCents int
	Priority   string
	Tags       []string
	Events     []Event
	Items      []LineItem
	// Delivery address fields; Street is stored encrypted at rest.
	DeliveryStreet string
	DeliveryCity   string
	DeliveryState  string
	DeliveryZIP    string
}

// Transition applies a state change and appends an immutable event.
// The caller must persist the event separately for durability; this
// function mutates only in-memory state.
func (o *Order) Transition(to Status, actor, reason, note string, at time.Time) (Event, error) {
	allowed, ok := transitions[o.Status]
	if !ok {
		return Event{}, fmt.Errorf("%w: terminal status %q", ErrBadTransition, o.Status)
	}
	if _, ok := allowed[to]; !ok {
		return Event{}, fmt.Errorf("%w: %s -> %s", ErrBadTransition, o.Status, to)
	}
	if to == StatusRefunded && reason == "" {
		return Event{}, ErrNoReason
	}
	ev := Event{
		OrderID: o.ID,
		At:      at,
		From:    o.Status,
		To:      to,
		Actor:   actor,
		Reason:  reason,
		Note:    note,
	}
	o.Status = to
	o.UpdatedAt = at
	o.Events = append(o.Events, ev)
	return ev, nil
}

// CanTransitionTo reports whether a specific destination is reachable from
// the current status in one step.
func (o *Order) CanTransitionTo(to Status) bool {
	allowed, ok := transitions[o.Status]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

// Exception is a flag pushed into the exception queue for operator attention.
type Exception struct {
	OrderID     string
	Kind        string // "picking_timeout", "out_of_stock"
	DetectedAt  time.Time
	Description string
}

// DetectOutOfStock returns an exception when the order has one or more
// backordered line items. The detector is pure: it derives the flag from
// the order's own data (updated by inventory or picking workflows) so the
// exception queue can be repopulated after a restart without replaying an
// event stream. Returns nil when no items are backordered.
func DetectOutOfStock(o *Order, now time.Time) *Exception {
	if o == nil || len(o.Items) == 0 {
		return nil
	}
	var missing []string
	for _, it := range o.Items {
		if it.Backordered {
			missing = append(missing, it.SKU)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &Exception{
		OrderID:     o.ID,
		Kind:        "out_of_stock",
		DetectedAt:  now,
		Description: fmt.Sprintf("%d/%d line items backordered (%v)", len(missing), len(o.Items), missing),
	}
}

// MarkItemBackordered flips the Backordered flag on a named SKU and
// returns true if the order changed. The caller persists the mutation.
// An unknown SKU is a no-op returning false.
func (o *Order) MarkItemBackordered(sku string, backordered bool) bool {
	for i := range o.Items {
		if o.Items[i].SKU == sku && o.Items[i].Backordered != backordered {
			o.Items[i].Backordered = backordered
			return true
		}
	}
	return false
}

// DetectPickingTimeout returns an exception when an order has been in
// "picking" for longer than PickingTimeout without a status change. The
// `now` parameter is explicit so tests and audit replays are deterministic.
func DetectPickingTimeout(o *Order, now time.Time) *Exception {
	if o.Status != StatusPicking {
		return nil
	}
	if now.Sub(o.UpdatedAt) < PickingTimeout {
		return nil
	}
	return &Exception{
		OrderID:    o.ID,
		Kind:       "picking_timeout",
		DetectedAt: now,
		Description: fmt.Sprintf("Order %s has been in picking for %s (threshold %s)",
			o.ID, now.Sub(o.UpdatedAt).Truncate(time.Second), PickingTimeout),
	}
}

// OutOfStockPlan describes how to respond to an out-of-stock item: either
// cancel, wait for restock, or split the shipment (send what is available
// now and fulfill the remainder on a second shipment).
type OutOfStockPlan struct {
	SuggestSplit     bool
	AvailableItems   []string
	BackorderedItems []string
	Description      string
}

// PlanOutOfStock inspects which items are available and which are backordered
// and produces an operator-facing plan. The heuristic is intentionally simple
// and matches the product's "prompt staff to split" requirement: if at least
// one item is available AND at least one is backordered, suggest a split.
func PlanOutOfStock(available, backordered []string) OutOfStockPlan {
	plan := OutOfStockPlan{
		AvailableItems:   append([]string(nil), available...),
		BackorderedItems: append([]string(nil), backordered...),
	}
	switch {
	case len(backordered) == 0:
		plan.Description = "All items available; no split needed"
	case len(available) == 0:
		plan.Description = "No items available; hold or cancel the order"
	default:
		plan.SuggestSplit = true
		plan.Description = fmt.Sprintf(
			"Split recommended: ship %d available now, backorder %d",
			len(available), len(backordered),
		)
	}
	return plan
}
