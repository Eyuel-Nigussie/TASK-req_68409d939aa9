import type { OrderView } from "../api/client";

const ORDER = ["placed", "picking", "dispatched", "delivered", "received"];

// OrderTimeline renders a linear breadcrumb of statuses. The current step
// is highlighted as "active", earlier steps as "done", and terminal refund
// or cancel states are shown as "exception".
export function OrderTimeline({ order }: { order: OrderView }) {
  const status = order.Status;
  const reached = new Set<string>();
  for (const ev of order.Events ?? []) {
    if (ev.From) reached.add(ev.From);
    reached.add(ev.To);
  }
  reached.add(status);
  // "placed" is the canonical start state even if no event captured it.
  reached.add("placed");

  const isException = status === "refunded" || status === "canceled";

  return (
    <div className="timeline" role="group" aria-label="Order timeline">
      {ORDER.map((step) => {
        let cls = "step";
        if (step === status) cls += " active";
        else if (reached.has(step) && idx(step) < idx(status)) cls += " done";
        return (
          <span key={step} className={cls}>
            {step}
          </span>
        );
      })}
      {isException && <span className="step exception">{status}</span>}
    </div>
  );
}

function idx(s: string) {
  const i = ORDER.indexOf(s);
  return i < 0 ? Infinity : i;
}
