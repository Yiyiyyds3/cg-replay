// StageText —— 把后端下发的 BoxDTO 台词框画到舞台画面上。
//
// 后端只给「舞台坐标系」的几何（x/y/w/h/font_size 都是 stage_w×stage_h 里的像素），
// 前端的 <img> 会被 object-contain 等比缩放，所以这里按图片实际显示区域算出
// 一个 (offset, scale)，把舞台坐标映射成屏幕像素。ResizeObserver + load 事件
// 保证窗口缩放、换帧、换事件后位置都跟着变。
import { useEffect, useState } from "react";
import type { BoxDTO } from "@/lib/api";

type Layout = { x: number; y: number; scale: number };

export default function StageText({
  boxes,
  stageW,
  stageH,
  stageRef,
  imgRef,
}: {
  boxes: BoxDTO[];
  stageW: number;
  stageH: number;
  stageRef: React.RefObject<HTMLElement | null>;
  imgRef: React.RefObject<HTMLImageElement | null>;
}) {
  const [layout, setLayout] = useState<Layout>({ x: 0, y: 0, scale: 1 });

  useEffect(() => {
    const stage = stageRef.current;
    const img = imgRef.current;
    if (!stage || !img) return;

    const recalc = () => {
      const nw = img.naturalWidth || stageW;
      const nh = img.naturalHeight || stageH;
      const cw = img.clientWidth;
      const ch = img.clientHeight;
      if (!nw || !nh || !cw || !ch) return;
      const s = Math.min(cw / nw, ch / nh);
      const sr = stage.getBoundingClientRect();
      const ir = img.getBoundingClientRect();
      setLayout({
        // object-contain 会在 img 盒子里留边，把这段偏移补回去
        x: ir.left - sr.left + (cw - nw * s) / 2,
        y: ir.top - sr.top + (ch - nh * s) / 2,
        scale: s,
      });
    };

    recalc();
    const ro = new ResizeObserver(recalc);
    ro.observe(img);
    ro.observe(stage);
    img.addEventListener("load", recalc);
    window.addEventListener("resize", recalc);
    return () => {
      ro.disconnect();
      img.removeEventListener("load", recalc);
      window.removeEventListener("resize", recalc);
    };
  }, [stageRef, imgRef, stageW, stageH]);

  if (boxes.length === 0) return null;
  const { x: bx, y: by, scale } = layout;

  return (
    <div className="pointer-events-none absolute inset-0 overflow-hidden">
      {boxes.map((b) => {
        // HasGeom=false：脚本没给 [position]，用 player 的默认底部对话框兜底。
        const w = b.has_geom && b.w > 0 ? b.w : stageW * 0.9;
        const h = b.has_geom && b.h > 0 ? b.h : 210;
        const x = b.has_geom ? b.x : (stageW - w) / 2;
        const y = b.has_geom ? b.y : stageH - h - 32;
        const fs = Math.max(9, b.font_size * scale);
        const lh = (b.font_size + (b.line_spacing || 12)) * scale;
        const c = b.frame_color ?? { r: 0, g: 0, b: 0 };
        const alpha = (b.frame_opacity ?? 128) / 255;
        return (
          <div
            key={b.layer}
            className="absolute overflow-hidden"
            style={{
              left: bx + x * scale,
              top: by + y * scale,
              width: w * scale,
              height: h * scale,
              backgroundColor: `rgba(${c.r},${c.g},${c.b},${alpha})`,
              borderRadius: 6 * scale,
              padding: 16 * scale,
              color: "#fff",
              fontSize: fs,
              lineHeight: `${lh}px`,
              textShadow: "0 1px 3px rgba(0,0,0,.95)",
              whiteSpace: "pre-wrap",
            }}
          >
            {b.lines.map((ln, i) => (
              <div key={i}>
                {ln.speaker && <span style={{ color: "#ffd98a" }}>{ln.speaker}：</span>}
                <span>{ln.text}</span>
              </div>
            ))}
          </div>
        );
      })}
    </div>
  );
}
