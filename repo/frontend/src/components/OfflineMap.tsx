import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";

interface Region {
  id: string;
  vertices: Array<{ lat: number; lng: number }>;
}

interface Props {
  onPinned?: (loc: { lat: number; lng: number; valid: boolean; regionID?: string }) => void;
}

// OfflineMap renders a preloaded service-area polygon set in an SVG canvas
// without needing a tile server. The bounding box is computed from the
// current polygon set and the canvas is scaled into 1000x600 units.
//
// The user clicks to place a pin, the component calls /dispatch/validate-pin,
// and the pin flips red if outside every configured zone. This is the
// "immediate validation feedback" the product spec requires.
export function OfflineMap({ onPinned }: Props) {
  const [regions, setRegions] = useState<Region[]>([]);
  const [pin, setPin] = useState<{ lat: number; lng: number; valid: boolean; regionID?: string; reason?: string } | null>(null);
  // mapImage is the admin-configured raster that renders behind the
  // polygon overlay. Empty string => polygon-only view (safe fallback).
  const [mapImage, setMapImage] = useState<string>("");
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    api
      .listRegions()
      .then((r) =>
        setRegions(
          r.map((rr) => ({
            id: rr.Polygon.ID,
            vertices: rr.Polygon.Vertices.map((v) => ({ lat: v.Lat, lng: v.Lng })),
          })),
        ),
      )
      .catch(() => setRegions([]));
    // Map config is optional; a failing fetch just leaves mapImage empty.
    api
      .getMapConfig()
      .then((c) => setMapImage(c.map_image_url ?? ""))
      .catch(() => setMapImage(""));
  }, []);

  const bbox = computeBBox(regions);
  const width = 1000;
  const height = 600;

  function project(lat: number, lng: number) {
    if (!bbox) return { x: width / 2, y: height / 2 };
    // SVG y increases downward; latitude increases upward.
    const x = ((lng - bbox.minLng) / (bbox.maxLng - bbox.minLng)) * width;
    const y = height - ((lat - bbox.minLat) / (bbox.maxLat - bbox.minLat)) * height;
    return { x, y };
  }

  function unproject(x: number, y: number) {
    if (!bbox) return { lat: 0, lng: 0 };
    const lng = bbox.minLng + (x / width) * (bbox.maxLng - bbox.minLng);
    const lat = bbox.minLat + ((height - y) / height) * (bbox.maxLat - bbox.minLat);
    return { lat, lng };
  }

  async function onClick(evt: React.MouseEvent<SVGSVGElement>) {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const localX = ((evt.clientX - rect.left) / rect.width) * width;
    const localY = ((evt.clientY - rect.top) / rect.height) * height;
    const { lat, lng } = unproject(localX, localY);
    try {
      const res = await api.validatePin(lat, lng);
      const p = { lat, lng, valid: res.valid, regionID: res.region_id, reason: res.reason };
      setPin(p);
      onPinned?.({ lat, lng, valid: res.valid, regionID: res.region_id });
    } catch (e: unknown) {
      const p = { lat, lng, valid: false, reason: (e as Error).message };
      setPin(p);
      onPinned?.({ lat, lng, valid: false });
    }
  }

  return (
    <div>
      <div className="map">
        <svg
          ref={svgRef}
          viewBox={`0 0 ${width} ${height}`}
          preserveAspectRatio="xMidYMid meet"
          role="img"
          aria-label="Offline service-area map"
          onClick={onClick}
          style={{ width: "100%", height: "auto", cursor: "crosshair" }}
        >
          <rect x={0} y={0} width={width} height={height} fill="#0b1220" />
          {mapImage && (
            /*
             * Raster backdrop configured by an admin via
             * /api/admin/map-config. The SVG keeps its own coordinate
             * system so polygon geometry stays aligned even when the
             * image is swapped.
             */
            <image
              href={mapImage}
              x={0}
              y={0}
              width={width}
              height={height}
              preserveAspectRatio="xMidYMid slice"
              opacity={0.85}
              data-testid="map-backdrop"
            />
          )}
          {regions.map((r) => {
            const pts = r.vertices.map((v) => {
              const p = project(v.lat, v.lng);
              return `${p.x},${p.y}`;
            }).join(" ");
            return (
              <polygon key={r.id} points={pts} fill="rgba(37,99,235,0.25)" stroke="#60a5fa" strokeWidth={2} />
            );
          })}
        </svg>
        {pin && bbox && (() => {
          const p = project(pin.lat, pin.lng);
          const left = (p.x / width) * 100 + "%";
          const top = (p.y / height) * 100 + "%";
          return (
            <div
              className={`pin ${pin.valid ? "" : "invalid"}`}
              style={{ left, top }}
              title={pin.valid ? `Inside ${pin.regionID}` : pin.reason || "Outside service area"}
            />
          );
        })()}
      </div>
      {pin && (
        <div
          className={pin.valid ? "ok-banner" : "error-banner"}
          role="status"
          style={{ marginTop: "0.5rem" }}
        >
          {pin.valid
            ? `Pin accepted in region ${pin.regionID} (${pin.lat.toFixed(4)}, ${pin.lng.toFixed(4)})`
            : `Pin outside configured service area (${pin.lat.toFixed(4)}, ${pin.lng.toFixed(4)})`}
        </div>
      )}
    </div>
  );
}

function computeBBox(regions: Region[]) {
  if (!regions.length) return null;
  let minLat = Infinity,
    maxLat = -Infinity,
    minLng = Infinity,
    maxLng = -Infinity;
  for (const r of regions) {
    for (const v of r.vertices) {
      if (v.lat < minLat) minLat = v.lat;
      if (v.lat > maxLat) maxLat = v.lat;
      if (v.lng < minLng) minLng = v.lng;
      if (v.lng > maxLng) maxLng = v.lng;
    }
  }
  // Pad bounding box by 5% so pin clicks near the edge don't rest on the
  // polygon border when projected back.
  const padLat = Math.max((maxLat - minLat) * 0.05, 0.01);
  const padLng = Math.max((maxLng - minLng) * 0.05, 0.01);
  return {
    minLat: minLat - padLat,
    maxLat: maxLat + padLat,
    minLng: minLng - padLng,
    maxLng: maxLng + padLng,
  };
}
