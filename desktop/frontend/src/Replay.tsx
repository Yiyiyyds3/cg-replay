// 回放页（方案 B M3）：舞台 + 控制条 + 调试抽屉 D。
// 数据流：60ms 轮询 api.getFrame()（Go 端墙钟推进 + 画面指纹缓存），
// jpeg 非空才写 <img>.src。缺素材红横幅常驻（FR-12 不静默）。
// 交互：舞台点击/轻触推进（拖动不算、双击全屏），台词按原生几何叠在画面上
// （文字由前端写，底色/框图已在 Go 侧 JPEG 里），语音状态按帧显示。
// 快捷键：Space/Enter 点击推进，D 调试抽屉，F 截图，Esc 返回，
// A 自动推进，M 静音，←/→ ±5s，P 播放/暂停。

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
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
  Mic,
  Maximize2,
  Minimize2,
  FileImage,
  FileVideo,
  Star,
  X,
} from "lucide-react";
import { api, onExportEvents, type FrameDTO, type SessionMeta } from "@/lib/api";
import SpineStage from "@/components/SpineStage";

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
  const [exportFrame, setExportFrame] = useState(0);
  const [exportTotal, setExportTotal] = useState(0);
  const [expMsg, setExpMsg] = useState(""); // 导出结果（路径 + 摘要，非静默）
  const [fav, setFav] = useState(false); // 本事件收藏状态（星标）
  const [favBusy, setFavBusy] = useState(false);
  const isSpine = !!meta.spine; // 骨骼动画：画面走 WebGL，Go 侧没有 JPEG/截图/导出

  // ---------- CG 剧情 / 鉴赏模式（Go 侧两套台本，切换只换播放器） ----------
  const [mode, setMode] = useState<string>(meta.mode ?? "");
  const [dialogue, setDialogue] = useState(meta.dialogue);
  const [modeBusy, setModeBusy] = useState(false);

  const switchMode = async (next: "story" | "gallery") => {
    if (modeBusy || mode === next) return;
    setModeBusy(true);
    try {
      const nm = await api.setReplayMode(next);
      setMode(nm.mode ?? next);
      setDialogue(nm.dialogue ?? dialogue);
      setErr("");
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    } finally {
      setModeBusy(false);
    }
  };

  // ---------- 舞台点击推进 / 拖动排除 / 双击全屏 ----------
  const clickTimer = useRef<number | null>(null);
  const dragRef = useRef({ x: 0, y: 0, moved: false });
  const [isFs, setIsFs] = useState(false);

  useEffect(() => {
    const onFs = () => setIsFs(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFs);
    return () => document.removeEventListener("fullscreenchange", onFs);
  }, []);

  // 离开回放时取消还没触发的单击定时器，避免退出瞬间又把下一次会话推进一格。
  useEffect(() => {
    return () => {
      if (clickTimer.current != null) {
        window.clearTimeout(clickTimer.current);
        clickTimer.current = null;
      }
    };
  }, []);

  const advance = async () => {
    try {
      await api.inputClick();
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    }
  };

  const toggleFullscreen = async () => {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await document.documentElement.requestFullscreen();
    } catch (e: any) {
      setErr(`全屏切换失败：${e?.message ?? String(e)}`);
    }
  };

  const onStagePointerDown = (e: ReactPointerEvent) => {
    dragRef.current = { x: e.clientX, y: e.clientY, moved: false };
  };
  const onStagePointerMove = (e: ReactPointerEvent) => {
    const d = dragRef.current;
    if (!d.moved && (Math.abs(e.clientX - d.x) > 8 || Math.abs(e.clientY - d.y) > 8)) {
      d.moved = true; // 拖动/滑过（去拖进度条、抽屉、选文字）不算点击推进
    }
  };
  const onStageClick = (e: ReactMouseEvent) => {
    if (dragRef.current.moved) return;
    if (e.detail > 1) return; // 双击的第二下不推进
    if (clickTimer.current != null) return;
    // 稍等 180ms 再推进：若紧接着来 dblclick 就取消（双击=全屏，不误触两次推进）。
    clickTimer.current = window.setTimeout(() => {
      clickTimer.current = null;
      void advance();
    }, 180);
  };
  const onStageDoubleClick = (e: ReactMouseEvent) => {
    e.preventDefault();
    if (clickTimer.current != null) {
      window.clearTimeout(clickTimer.current);
      clickTimer.current = null;
    }
    void toggleFullscreen();
  };

  // 进入回放：读收藏列表定位本事件星标（列表小，一次拉全）。
  useEffect(() => {
    let alive = true;
    void api
      .listFavorites("event")
      .then((items) => {
        if (!alive) return;
        setFav(items.some((x) => x.game_id === meta.game_id && x.item_id === meta.id));
      })
      .catch(() => void 0); // 收藏查询失败不阻断回放（星标只是附加功能）
    return () => {
      alive = false;
    };
  }, [meta.game_id, meta.id]);

  const toggleFav = async () => {
    setFavBusy(true);
    const had = fav;
    setFav(!had); // 乐观更新
    try {
      if (had) await api.removeFavorite("event", meta.game_id, meta.id);
      else await api.addFavorite("event", meta.game_id, meta.id, 0, "");
    } catch (e: any) {
      setFav(had); // 回滚
      setErr(`${had ? "取消收藏" : "收藏"}失败：${e?.message ?? String(e)}`);
    } finally {
      setFavBusy(false);
    }
  };

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
    void seekStep(Math.round(pct * dur));
  };

  // 快捷键
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.repeat) return;
      // 弹层打开时 Esc 归弹层（否则会顺手把回放标签一起关掉）。
      if (
        e.key === "Escape" &&
        document.querySelector('[role="dialog"][aria-modal="true"]')
      ) {
        return;
      }
      // 焦点在输入类控件里时不抢按键（进度条方向键交给原生控件）。
      const el = e.target as HTMLElement | null;
      if (
        e.key !== "Escape" &&
        el &&
        (el.tagName === "INPUT" ||
          el.tagName === "TEXTAREA" ||
          el.tagName === "SELECT" ||
          el.isContentEditable)
      ) {
        return;
      }
      switch (e.key) {
        case " ":
        case "Enter":
          // 原作语义：空格/回车 = 点击推进（不是播放开关；播放开关在 P）。
          e.preventDefault();
          void advance();
          break;
        case "p":
        case "P":
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
    if (isSpine) {
      // 画面在 WebGL canvas 里，Go 侧没有可截的帧；明确告知而不是静默无反应。
      setErr("Spine 骨骼动画暂不支持截图（画面由前端 WebGL 渲染）");
      return;
    }
    try {
      const p = await api.screenshot();
      console.info("[截图]", p);
      setShot(p);
      window.setTimeout(() => setShot(""), 4000);
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    }
  };

  // 导出任务中心事件（Go 端 EventsEmit；progress/done/failed 三路）。
  // done/failed 都要清 running 态；「导出已取消」走 info toast 而非错误横幅
  // （用户主动取消不是故障，FR-12 不静默但不恐吓）。
  useEffect(
    () =>
      onExportEvents({
        onProgress: (p) => {
          setExportPct(p?.percent ?? 0);
          setExportFrame(p?.frame ?? 0);
          setExportTotal(p?.total ?? 0);
        },
        onDone: (r) => {
          setExporting("");
          setExportPct(0);
          setExpMsg(
            `已导出 ${r.format.toUpperCase()}：${r.path}（${r.frames} 帧 / ${r.fps}fps / ${r.width}×${r.height}）` +
              (r.note ? ` · ${r.note}` : ""),
          );
          window.setTimeout(() => setExpMsg(""), 12000);
        },
        onFailed: (msg) => {
          setExporting("");
          setExportPct(0);
          if (msg.includes("已取消")) {
            setExpMsg(msg);
            window.setTimeout(() => setExpMsg(""), 6000);
          } else {
            setErr(msg);
          }
        },
      }),
    [],
  );

  const doExport = async (format: "gif" | "mp4") => {
    if (exporting || isSpine) return;
    setExporting(format);
    setExportPct(0);
    setExportFrame(0);
    setExportTotal(0);
    try {
      await api.startExport(format); // 异步：进度/结果/失败走事件
    } catch (e: any) {
      // 同步校验失败（并发任务/无会话/无 ffmpeg）——立即回到可操作态
      setExporting("");
      setErr(e?.message ?? String(e));
    }
  };

  const cancelExport = async () => {
    try {
      await api.cancelExport();
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    }
  };

  // 进度条刻度：脚本时钟最大值（dur_ms），可能为 0（无 wait/wt 的事件）。
  // 剧情/鉴赏切换后台本变了，时长以帧里的 dur_ms 为准（会话内实时值）。
  const dur = f?.dur_ms ?? meta.duration_ms ?? 0;
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

  // 舞台台词覆盖层：直接用 SVG viewBox 贴原生舞台坐标（0 0 stage_w stage_h），
  // 与 object-contain 的 JPEG 共用同一块「等比内切矩形」，窗口缩放/黑边都不跑位。
  // 文字必须由前端画——Go 侧 RenderFrame 只画对话框底色/框图（win3.png），
  // 文字原本交给平台层 GDI，Web 前端这里用 SVG text。
  const stageTexts = useMemo(() => {
    const boxes = (f?.texts ?? []).filter(
      (b) => b.visible && b.lines.length > 0 && b.has_geom && b.w > 0 && b.h > 0,
    );
    // 有名字牌（message1）时，台词行不再重复说话人（原名牌已经显示）。
    const hasPlate = boxes.some((b) => b.layer === "message1");
    return (
      <svg
        className="pointer-events-none absolute inset-0 h-full w-full"
        viewBox={`0 0 ${meta.stage_w} ${meta.stage_h}`}
        preserveAspectRatio="xMidYMid meet"
        aria-hidden="true"
      >
        {boxes.map((b) => {
          const x = b.x + b.margin_l;
          let y = b.y + b.margin_t;
          return (
            <g key={b.layer} style={{ fontFamily: '"Noto Sans CJK SC","Microsoft YaHei",sans-serif' }}>
              {b.lines.map((ln, i) => {
                y += b.font_size; // 本行基线（行距 = 字号 + line_spacing）
                const lineY = y;
                y += b.line_spacing;
                const showSpeaker = b.layer !== "message1" && !!ln.speaker && !hasPlate;
                return (
                  <text
                    key={i}
                    x={x}
                    y={lineY}
                    fontSize={b.font_size}
                    fill="#fff"
                    stroke="rgba(0,0,0,0.85)"
                    strokeWidth={2}
                    paintOrder="stroke"
                    xmlSpace="preserve"
                  >
                    {showSpeaker && (
                      <tspan opacity={0.8}>{ln.speaker}：</tspan>
                    )}
                    <tspan>{ln.text}</tspan>
                  </text>
                );
              })}
            </g>
          );
        })}
      </svg>
    );
  }, [f?.texts, meta.stage_w, meta.stage_h]);

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
        {/* CG 剧情 / 鉴赏切换：Go 侧两套台本都在内存里，切换只重建播放器 */}
        {meta.kind === "cg" && (meta.modes?.length ?? 0) >= 2 && (
          <div className="flex overflow-hidden rounded border border-border text-xs">
            <button
              type="button"
              disabled={modeBusy}
              onClick={() => void switchMode("story")}
              className={
                "px-2 py-1 transition-colors " +
                (mode === "story"
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-secondary")
              }
              title="剧情模式：主线原演出（含台词与语音）"
            >
              剧情
            </button>
            <button
              type="button"
              disabled={modeBusy}
              onClick={() => void switchMode("gallery")}
              className={
                "px-2 py-1 transition-colors " +
                (mode === "gallery"
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-secondary")
              }
              title="鉴赏模式：画廊原段（纯图，可翻页）"
            >
              鉴赏
            </button>
          </div>
        )}
        {dialogue?.available && (
          <Badge variant="outline" title={`剧情章节：${dialogue.chapter ?? "—"}`}>
            台词 {dialogue.text_lines} · 语音 {dialogue.voice_lines}
          </Badge>
        )}
        <Button
          variant="ghost"
          size="icon"
          disabled={favBusy}
          onClick={() => void toggleFav()}
          title={fav ? "取消收藏（星标）" : "收藏本事件（星标）"}
          className={fav ? "text-amber-400" : "text-muted-foreground"}
        >
          <Star className={fav ? "h-5 w-5 fill-current" : "h-5 w-5"} />
        </Button>
        {showDbg && (
          <>
            <Badge variant="outline">{meta.op_count} op</Badge>
            <Badge variant="outline">文本 {meta.text_count} 段</Badge>
          </>
        )}
        {!isSpine && (
          <>
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
          </>
        )}
        {exporting && (
          <>
            <Badge
              variant="outline"
              className="tabular-nums"
              title={`第 ${exportFrame}/${exportTotal} 帧`}
            >
              {exporting.toUpperCase()} {exportPct}%
              {exportTotal > 0 && ` · ${exportFrame}/${exportTotal}`}
            </Badge>
            <Button
              variant="ghost"
              size="icon"
              title="取消导出"
              onClick={() => void cancelExport()}
            >
              <X className="h-4 w-4" />
            </Button>
          </>
        )}
        <Button
          variant="ghost"
          size="icon"
          onClick={() => void toggleFullscreen()}
          title={isFs ? "退出全屏（双击画面也可）" : "全屏（双击画面也可）"}
        >
          {isFs ? <Minimize2 className="h-5 w-5" /> : <Maximize2 className="h-5 w-5" />}
        </Button>
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

      {/* 渲染失败常驻横幅：影片取帧/图层解码错误不再被吞 */}
      {f && f.drawn?.some((d) => d.startsWith("✗")) && (
        <div className="border-b border-amber-900 bg-amber-950/80 px-4 py-1.5 text-xs text-amber-300">
          {f.drawn.filter((d) => d.startsWith("✗")).join("；")}
        </div>
      )}
      {/* 缺素材常驻红横幅（FR-12：不静默） */}
      {f && f.missing.length > 0 && (
        <div className="border-b border-red-900 bg-red-950/80 px-4 py-1.5 text-xs text-red-300">
          缺素材 {f.missing.length} 个：{f.missing.join("、")}—— 对应图层以
          1×1 透明占位继续合成，可跳过该事件或补素材后重扫。
        </div>
      )}
      {/* CG 台词数据横幅：有剧情就提示可切；真没有就如实说，不假装有 */}
      {meta.kind === "cg" && dialogue && !dialogue.available && (
        <div className="border-b border-amber-900 bg-amber-950/80 px-4 py-1.5 text-xs text-amber-300">
          {dialogue.note ?? "该 CG 无台词数据"}
          。当前以鉴赏模式播放（仅图片，无文本/语音）。
        </div>
      )}
      {meta.kind === "cg" && dialogue?.available && mode === "gallery" && (
        <div className="border-b border-border bg-secondary/60 px-4 py-1.5 text-xs text-muted-foreground">
          当前为鉴赏模式（纯图，可能多页翻页）。切到「剧情」可看这段 CG 在
          {dialogue.chapter ? `「${dialogue.chapter}」` : "主线"}
          里的原始演出：台词 {dialogue.text_lines} 条 · 语音 {dialogue.voice_lines} 句。
        </div>
      )}
      {/* 会话错误红横幅 */}
      {err && (
        <div className="border-b border-red-900 bg-red-950/80 px-4 py-1.5 text-xs text-red-300">
          {err}
        </div>
      )}
      {/* Spine 素材/统计警告：导出器有 problems 时不能静默 */}
      {meta.spine?.warnings?.length ? (
        <div className="border-b border-amber-900 bg-amber-950/80 px-4 py-1.5 text-xs text-amber-300">
          Spine 提示：{meta.spine.warnings.join("；")}
        </div>
      ) : null}
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

      {/* 舞台：画面 + 台词覆盖层。点击/轻触推进（拖动、按钮、抽屉都不触发），双击全屏 */}
      <main
        className={
          "relative flex flex-1 items-center justify-center overflow-hidden bg-black " +
          (f?.waiting ? "cursor-pointer" : "cursor-default")
        }
        style={{ touchAction: "manipulation" }}
        onPointerDown={onStagePointerDown}
        onPointerMove={onStagePointerMove}
        onClick={onStageClick}
        onDoubleClick={onStageDoubleClick}
        title={f?.waiting ? "点击继续（双击全屏）" : "点击推进（双击全屏）"}
      >
        <div
          className="flex h-full max-w-full items-center justify-center"
          style={{
            transform: `translate(${f?.quake_x ?? 0}px, ${f?.quake_y ?? 0}px)`,
          }}
        >
          <div className="relative flex h-full w-full items-center justify-center">
            {meta.spine ? (
              <SpineStage
                spine={meta.spine}
                now={f?.now ?? 0}
                playing={!!f?.playing}
                done={!!f?.done}
                onError={setErr}
              />
            ) : (
              <>
                <img
                  ref={imgRef}
                  alt={meta.title}
                  className="h-full w-full object-contain"
                  draggable={false}
                />
                {/* 台词（说话人 + 文本）：SVG 覆盖，pointer-events-none 不挡点击 */}
                {stageTexts}
              </>
            )}
          </div>
        </div>
        {/* 点击推进提示 + 等待态 */}
        <div className="pointer-events-none absolute bottom-3 right-3 rounded bg-black/60 px-2 py-1 text-[11px] text-white/85 shadow">
          {f?.waiting ? "点击画面继续 ▸" : "点击画面推进 · 双击全屏"}
        </div>
        {f?.waiting && (
          <span className="pointer-events-none absolute bottom-3 left-3 inline-flex items-center gap-1 rounded bg-black/60 px-2 py-1 text-[11px] text-amber-200">
            <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-amber-300" />
            等待推进
          </span>
        )}
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
        {/* 语音状态：只报「最近触发的一句」，不谎报播放进度/时长 */}
        {f?.voice && (
          <Badge
            variant={f.voice.muted ? "outline" : "secondary"}
            title={`本句语音：${f.voice.ref}（时长由素材决定；静音时仍随帧推进）`}
          >
            {f.voice.muted ? (
              <VolumeX className="mr-1 h-3 w-3" />
            ) : (
              <Mic className="mr-1 h-3 w-3" />
            )}
            语音
          </Badge>
        )}
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
