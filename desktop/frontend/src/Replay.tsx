// 回放页（方案 B M3）：舞台 + 控制条 + 调试抽屉 D。
// 数据流：60ms 轮询 api.getFrame()（Go 端墙钟推进 + 画面指纹缓存），
// jpeg 非空才写 <img>.src。缺素材红横幅常驻（FR-12 不静默）。
// 快捷键：Space/Enter 点击推进，D 调试抽屉，F 截图，Esc 返回，
// A 自动推进，M 静音，←/→ ±5s，P 播放/暂停。

import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  ChevronLeft,
  Camera,
  Bug,
  Images,
  SkipBack,
  SkipForward,
  Play,
  Pause,
  Volume2,
  VolumeX,
  FileImage,
  FileVideo,
} from "lucide-react";
import { api, type FrameDTO, type SessionMeta } from "@/lib/api";

const KIND_LABEL: Record<string, string> = {
  cg: "CG 鉴赏",
  scene: "主线场景",
  chapter: "演出段落",
};

export default function Replay({
  meta,
  onExit,
}: {
  meta: SessionMeta;
  onExit: () => void;
}) {
  const [f, setF] = useState<FrameDTO | null>(null);
  const [err, setErr] = useState("");
  const [showDbg, setShowDbg] = useState(false);
  const [showInsp, setShowInsp] = useState(false);
  const imgRef = useRef<HTMLImageElement | null>(null);
  const [shot, setShot] = useState(""); // 最近一次截图保存路径提示
  const [exporting, setExporting] = useState<"" | "gif" | "mp4">("");
  const [exportPct, setExportPct] = useState(0);
  const [expMsg, setExpMsg] = useState(""); // 导出结果（路径 + 摘要，非静默）

  // 会话关闭不做在卸载清理里：React 18 dev StrictMode 双挂载
  // （mount→unmount→remount）会在首次卸载把正常会话关掉。
  // 显式返回（返回按钮/Esc → App.onExit）时才 exitReplay。

  // 轮询：Go 端是唯一时钟（墙钟推进），前端只拉帧。
  // 非 setInterval：上一帧未回不发下一帧（避免重叠请求把
  // 编码队列堆成无上界）；间隔固定 60ms 节拍，忙时自然放慢。
  useEffect(() => {
    let alive = true;
    const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
    const loop = async () => {
      while (alive) {
        const t0 = Date.now();
        let nf: FrameDTO;
        try {
          nf = await api.getFrame();
        } catch (e: any) {
          setErr(e?.message ?? String(e));
          await sleep(150);
          continue;
        }
        if (!alive) return;
        setErr("");
        setF(nf);
        if (nf.jpeg && nf.jpeg !== imgRef.current?.src && imgRef.current) {
          imgRef.current.src = nf.jpeg;
        }
        await sleep(Math.max(10, 60 - (Date.now() - t0)));
      }
    };
    void loop();
    return () => {
      alive = false;
    };
  }, []);

  const seekStep = async (ms: number) => {
    const base = f?.now ?? 0;
    try {
      await api.seekTo(Math.max(0, base + ms));
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    }
  };

  const onSeek = (pct: number) => {
    void seekStep(Math.round(pct * (meta.duration_ms || 0)));
  };

  // 快捷键
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.repeat) return;
      switch (e.key) {
        case " ":
        case "Enter":
          e.preventDefault();
          void api.togglePlay().catch((e) => setErr(e?.message ?? String(e)));
          break;
        case "d":
        case "D":
          setShowDbg((v) => !v);
          break;
        case "i":
        case "I":
          setShowInsp((v) => !v);
          break;
        case "f":
        case "F":
          screenshot();
          break;
        case "Escape":
          onExit();
          break;
        case "a":
        case "A":
          void api.toggleAuto().catch((e) => setErr(e?.message ?? String(e)));
          break;
        case "m":
        case "M":
          void api
            .setMuted(!(f?.muted ?? false))
            .catch((e) => setErr(e?.message ?? String(e)));
          break;
        case "ArrowLeft":
          void seekStep(-5000);
          break;
        case "ArrowRight":
          void seekStep(5000);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [f?.muted, f?.now]);

  const screenshot = async () => {
    try {
      const p = await api.screenshot();
      console.info("[截图]", p);
      setShot(p);
      window.setTimeout(() => setShot(""), 4000);
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    }
  };

  // 导出进度（Go 端 wruntime.EventsEmit；无 runtime 时静默跳过）
  useEffect(() => {
    const rt = (window as any).runtime;
    if (!rt?.EventsOn) return;
    const off = rt.EventsOn("export:progress", (p: any) =>
      setExportPct(p?.percent ?? 0),
    );
    return () => {
      if (typeof off === "function") off();
    };
  }, []);

  const doExport = async (format: "gif" | "mp4") => {
    if (exporting) return;
    setExporting(format);
    setExportPct(0);
    try {
      const r = await api.exportEvent(format);
      setExpMsg(
        `已导出 ${r.format.toUpperCase()}：${r.path}（${r.frames} 帧 / ${r.fps}fps / ${r.width}×${r.height}）` +
          (r.note ? ` · ${r.note}` : ""),
      );
      window.setTimeout(() => setExpMsg(""), 12000);
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    } finally {
      setExporting("");
    }
  };

  // 进度条刻度：脚本时钟最大值（dur_ms），可能为 0（无 wait/wt 的事件）
  const dur = meta.duration_ms || 0;
  const pct = dur > 0 ? Math.min(1, (f?.now ?? 0) / dur) : 0;

  // 资产检视器数据：ref 去重 + 使用状态（缺素材优先、未引用过的不出现）
  const assetRefs = useMemo(() => {
    const m = new Map<string, { ref: string; visible: boolean; missing: boolean }>();
    for (const l of f?.layers ?? []) {
      if (!l.ref) continue;
      const cur = m.get(l.ref);
      const vis = cur?.visible || l.visible;
      const mis = cur?.missing || l.missing;
      m.set(l.ref, { ref: l.ref, visible: vis, missing: mis });
    }
    const arr = [...m.values()];
    arr.sort((a, b) =>
      a.missing !== b.missing ? (a.missing ? -1 : 1) : a.ref.localeCompare(b.ref),
    );
    return arr;
  }, [f]);

  return (
    <div className="relative flex h-screen flex-col bg-background text-foreground">
      {/* 顶栏 */}
      <header className="flex items-center gap-3 border-b border-border bg-background px-4 py-2">
        <Button variant="ghost" size="icon" onClick={onExit} title="返回（Esc）">
          <ChevronLeft className="h-5 w-5" />
        </Button>
        <div className="min-w-0 flex-1 truncate text-sm font-medium">
          {meta.title}
        </div>
        <Badge variant="secondary">{KIND_LABEL[meta.kind] ?? meta.kind}</Badge>
        {showDbg && (
          <>
            <Badge variant="outline">{meta.op_count} op</Badge>
            <Badge variant="outline">文本 {meta.text_count} 段</Badge>
          </>
        )}
        <Button variant="ghost" size="icon" onClick={screenshot} title="截图（F）">
          <Camera className="h-5 w-5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          disabled={!!exporting}
          onClick={() => doExport("gif")}
          title="导出 GIF（10fps / 480×360 / 15s 上限）"
        >
          <FileImage className="h-5 w-5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          disabled={!!exporting}
          onClick={() => doExport("mp4")}
          title="导出 MP4（25fps / 960×720 / 60s 上限；需本机 ffmpeg）"
        >
          <FileVideo className="h-5 w-5" />
        </Button>
        {exporting && (
          <Badge variant="outline" className="tabular-nums">
            {exporting.toUpperCase()} 导出 {exportPct}%
          </Badge>
        )}
        <Button
          variant={showInsp ? "secondary" : "ghost"}
          size="icon"
          onClick={() => setShowInsp((v) => !v)}
          title="资产检视器（I）"
        >
          <Images className="h-5 w-5" />
        </Button>
        <Button
          variant={showDbg ? "secondary" : "ghost"}
          size="icon"
          onClick={() => setShowDbg((v) => !v)}
          title="调试抽屉（D）"
        >
          <Bug className="h-5 w-5" />
        </Button>
      </header>

      {/* 缺素材常驻红横幅（FR-12：不静默） */}
      {f && f.missing.length > 0 && (
        <div className="border-b border-red-900 bg-red-950/80 px-4 py-1.5 text-xs text-red-300">
          缺素材 {f.missing.length} 个：{f.missing.join("、")}—— 对应图层以
          1×1 透明占位继续合成，可跳过该事件或补素材后重扫。
        </div>
      )}
      {/* 会话错误红横幅 */}
      {err && (
        <div className="border-b border-red-900 bg-red-950/80 px-4 py-1.5 text-xs text-red-300">
          {err}
        </div>
      )}
      {/* 截图完成提示（路径可见，非静默） */}
      {shot && (
        <div className="absolute right-4 top-14 z-20 max-w-[80%] truncate rounded border border-border bg-card px-3 py-1.5 text-xs shadow">
          已保存：{shot}
        </div>
      )}
      {/* 导出完成提示（路径 + 帧率/尺寸 + 截断/缺素材说明） */}
      {expMsg && (
        <div className="absolute right-4 top-14 z-20 max-h-32 max-w-[85%] overflow-auto whitespace-pre-wrap rounded border border-border bg-card px-3 py-1.5 text-xs shadow">
          {expMsg}
        </div>
      )}

      {/* 舞台（4:3，1600×1200 画面） */}
      <main className="relative flex flex-1 items-center justify-center overflow-hidden bg-black">
        <div
          className="flex h-full max-w-full items-center justify-center"
          style={{
            transform: `translate(${f?.quake_x ?? 0}px, ${f?.quake_y ?? 0}px)`,
          }}
        >
          <img
            ref={imgRef}
            alt={meta.title}
            className="max-h-full max-w-full object-contain"
            draggable={false}
          />
        </div>
        {/* 点击推进（对齐原作「点击继续」） */}
        <div
          className="absolute inset-0 cursor-pointer"
          onClick={() => api.inputClick().catch((e) => setErr(e?.message ?? String(e)))}
        />
      </main>

      {/* 控制条 */}
      <footer className="flex items-center gap-3 border-t border-border bg-background px-4 py-2">
        <span className="tabular-nums text-xs text-muted-foreground">
          {fmt(f?.now)} / {fmt(dur)}
        </span>
        <input
          type="range"
          min={0}
          max={1000}
          value={Math.round(pct * 1000)}
          onChange={(e) => onSeek(Number(e.target.value) / 1000)}
          className="h-1.5 flex-1 cursor-pointer appearance-none rounded bg-secondary accent-[hsl(var(--primary))]"
          disabled={dur <= 0}
          title={dur <= 0 ? "该事件没有时间轴刻度" : "跳转"}
        />
        <Button variant="ghost" size="icon" onClick={() => seekStep(-5000)} title="后退 5s（←）">
          <SkipBack className="h-4 w-4" />
        </Button>
        <Button
          variant="default"
          size="icon"
          onClick={() => api.togglePlay().catch((e) => setErr(e?.message ?? String(e)))}
          title="播放/暂停（空格）"
        >
          {f?.playing ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
        </Button>
        <Button variant="ghost" size="icon" onClick={() => seekStep(5000)} title="快进 5s（→）">
          <SkipForward className="h-4 w-4" />
        </Button>
        <Button
          variant={f?.auto ? "secondary" : "ghost"}
          size="sm"
          onClick={() => api.toggleAuto().catch((e) => setErr(e?.message ?? String(e)))}
          title="自动推进（A）"
        >
          自动
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => api.setMuted(!(f?.muted ?? false)).catch((e) => setErr(e?.message ?? String(e)))}
          title="静音（M）"
        >
          {f?.muted ? <VolumeX className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}
        </Button>
        {f?.waiting && <Badge variant="warn">等待推进</Badge>}
        {f?.done && <Badge variant="outline">结束</Badge>}
      </footer>

      {/* 调试抽屉 D */}
      {showDbg && (
        <aside className="fixed right-0 top-0 z-40 h-screen w-96 overflow-y-auto border-l border-border bg-background/95 p-3 text-xs shadow-2xl">
          <div className="mb-2 flex items-center justify-between">
            <span className="font-medium">调试抽屉</span>
            <Button variant="ghost" size="sm" onClick={() => setShowDbg(false)}>
              关闭
            </Button>
          </div>

          {/* 图层表（z 升序） */}
          <div className="mb-1 font-medium text-muted-foreground">图层（z 升序）</div>
          <table className="mb-3 w-full table-fixed">
            <thead>
              <tr className="text-left text-muted-foreground">
                <th className="w-24">layer</th>
                <th>ref</th>
                <th className="w-10 text-right">z</th>
                <th className="w-10 text-right">α</th>
              </tr>
            </thead>
            <tbody>
              {(f?.layers ?? []).map((l) => (
                <tr key={l.layer} className={l.visible ? "" : "opacity-40"}>
                  <td className="truncate">{l.layer}</td>
                  <td className={`truncate ${l.missing ? "text-red-400" : ""}`}>
                    {l.ref}
                    {l.missing ? "（缺）" : ""}
                  </td>
                  <td className="text-right">{l.z}</td>
                  <td className="text-right">{Math.round(l.opacity * 100)}%</td>
                </tr>
              ))}
            </tbody>
          </table>

          {/* 文本框（播放器实时行） */}
          <div className="mb-1 font-medium text-muted-foreground">文本框</div>
          {(f?.texts ?? []).length === 0 && (
            <div className="mb-3 text-muted-foreground">（当前无可见文本）</div>
          )}
          {(f?.texts ?? []).map((t) => (
            <div key={t.layer} className="mb-2 rounded border border-border p-2">
              <div className="mb-1 flex items-center justify-between text-muted-foreground">
                <span>{t.layer}</span>
                <span>
                  {Math.round(t.x)},{Math.round(t.y)} · {int(t.font_size)}px{" "}
                  {t.frame_opacity ? `底α${t.frame_opacity}` : ""}
                </span>
              </div>
              {t.lines.map((ln, i) => (
                <div key={i} className="leading-5">
                  {ln.speaker && (
                    <span className="text-muted-foreground">{ln.speaker}：</span>
                  )}
                  <span className="text-sm">{ln.text}</span>
                </div>
              ))}
            </div>
          ))}

          {/* 会话元信息 */}
          <div className="mt-3 border-t border-border pt-2 text-muted-foreground">
            <div>脚本时序 op：{meta.op_count} · 文本段：{meta.text_count}</div>
            <div>刻度时长：{fmt(dur)}（速度=播放器策略，非脚本数据）</div>
            <div>帧：{f ? `${fmt(f.now)} · 缓存${f.cached ? "✓" : "✗"}` : "…"}</div>
          </div>
        </aside>
      )}

      {/* 资产检视器 I：本会话引用过的素材 ref + 使用状态（E-mote 标注沿用徽标语义） */}
      {showInsp && (
        <aside className="fixed right-0 top-0 z-30 h-screen w-96 overflow-y-auto border-l border-border bg-background/95 p-3 text-xs shadow-2xl">
          <div className="mb-2 flex items-center justify-between">
            <span className="font-medium">资产检视器</span>
            <Button variant="ghost" size="sm" onClick={() => setShowInsp(false)}>
              关闭
            </Button>
          </div>
          <div className="mb-2 text-muted-foreground">
            舞台 {meta.stage_w}×{meta.stage_h} · 引用 {assetRefs.length} 个素材
          </div>
          <ul className="space-y-1">
            {assetRefs.map((r) => (
              <li key={r.ref} className="flex items-center gap-2">
                <span
                  className={
                    "inline-block h-1.5 w-1.5 shrink-0 rounded-full " +
                    (r.missing ? "bg-red-500" : r.visible ? "bg-emerald-500" : "bg-muted-foreground/40")
                  }
                />
                <span className={"truncate " + (r.missing ? "text-red-400" : "")}>{r.ref}</span>
                <span className="ml-auto shrink-0 text-muted-foreground">
                  {r.missing ? "缺" : r.visible ? "在用" : "后备"}
                </span>
              </li>
            ))}
          </ul>
          {f && f.missing.length > 0 && (
            <div className="mt-3 border-t border-border pt-2 text-red-400">
              当前帧缺 {f.missing.length} 个素材（回放横幅同步常驻）。
            </div>
          )}
        </aside>
      )}
    </div>
  );
}

function fmt(ms?: number) {
  const s = Math.max(0, Math.round((ms ?? 0) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

function int(n?: number) {
  return Math.round(n ?? 0);
}
