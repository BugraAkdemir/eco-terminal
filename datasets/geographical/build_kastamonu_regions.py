#!/usr/bin/env python3
"""Build kastamonu_regions.geojson from geoBoundaries gbOpen TUR ADM1/ADM2 (OpenStreetMap-derived, ODbL).

Usage: build_kastamonu_regions.py ADM1.geojson ADM2.geojson OUT.geojson
Selects the Kastamonu province (ADM1) and its 20 districts (ADM2, by name + bbox sanity check),
simplifies rings with Douglas-Peucker (0.0004 deg ~ 40 m) and rounds to 5 decimals.
See docs/data_sources/regions.md for URLs, license and retrieval date.
"""
import json
import sys

DISTRICTS = ["Kastamonu merkez", "Abana", "Ağlı", "Araç", "Azdavay", "Bozkurt", "Cide", "Çatalzeytin", "Daday",
             "Devrekani", "Doğanyurt", "Hanönü", "İhsangazi", "İnebolu", "Küre", "Pınarbaşı", "Seydiler",
             "Şenpazar", "Taşköprü", "Tosya"]
TOL = 0.0004
PROV_BBOX = (32.4, 40.5, 35.5, 42.2)  # generous sanity bbox for Kastamonu province


def dp(pts, tol):
    if len(pts) < 5:
        return pts
    keep = [False] * len(pts)
    keep[0] = keep[-1] = True
    stack = [(0, len(pts) - 1)]
    while stack:
        a, b = stack.pop()
        (x1, y1), (x2, y2) = pts[a][:2], pts[b][:2]
        dx, dy = x2 - x1, y2 - y1
        length2 = dx * dx + dy * dy
        mx, mi = -1.0, -1
        for i in range(a + 1, b):
            x, y = pts[i][:2]
            if length2 == 0:
                d = ((x - x1) ** 2 + (y - y1) ** 2) ** 0.5
            else:
                t = max(0, min(1, ((x - x1) * dx + (y - y1) * dy) / length2))
                d = ((x - (x1 + t * dx)) ** 2 + (y - (y1 + t * dy)) ** 2) ** 0.5
            if d > mx:
                mx, mi = d, i
        if mx > tol:
            keep[mi] = True
            stack += [(a, mi), (mi, b)]
    return [p for p, k in zip(pts, keep) if k]


def simp_ring(r):
    s = dp(r, TOL)
    s = [[round(p[0], 5), round(p[1], 5)] for p in s]
    if s[0] != s[-1]:
        s.append(s[0])
    return s if len(s) >= 4 else None


def simp_geom(g):
    polys = [g["coordinates"]] if g["type"] == "Polygon" else g["coordinates"]
    out = []
    for poly in polys:
        rings = [simp_ring(r) for r in poly]
        rings = [r for r in rings if r]
        if rings:
            out.append(rings)
    if len(out) == 1:
        return {"type": "Polygon", "coordinates": out[0]}
    return {"type": "MultiPolygon", "coordinates": out}


def bbox(g):
    polys = [g["coordinates"]] if g["type"] == "Polygon" else g["coordinates"]
    xs = [p[0] for poly in polys for p in poly[0]]
    ys = [p[1] for poly in polys for p in poly[0]]
    return min(xs), min(ys), max(xs), max(ys)


def inside(b):
    return PROV_BBOX[0] < b[0] and b[2] < PROV_BBOX[2] and PROV_BBOX[1] < b[1] and b[3] < PROV_BBOX[3]


def main():
    adm1 = json.load(open(sys.argv[1]))["features"]
    adm2 = json.load(open(sys.argv[2]))["features"]
    feats = []
    prov = [f for f in adm1 if f["properties"]["shapeName"] == "Kastamonu"]
    assert len(prov) == 1
    feats.append({"type": "Feature",
                  "properties": {"kind": "province", "name": "Kastamonu", "shapeID": prov[0]["properties"]["shapeID"]},
                  "geometry": simp_geom(prov[0]["geometry"])})
    for name in DISTRICTS:
        c = [f for f in adm2 if f["properties"]["shapeName"] == name and inside(bbox(f["geometry"]))]
        assert len(c) == 1, (name, len(c))
        f = c[0]
        feats.append({"type": "Feature",
                      "properties": {"kind": "district", "name": "Merkez" if name == "Kastamonu merkez" else name,
                                     "shapeID": f["properties"]["shapeID"]},
                      "geometry": simp_geom(f["geometry"])})
    with open(sys.argv[3], "w") as fh:
        json.dump({"type": "FeatureCollection", "features": feats}, fh, separators=(",", ":"))
    print(len(feats), "features")


main()
