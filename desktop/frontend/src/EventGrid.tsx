import { useEffect, useMemo, useRef, useState } from "react";
import {
  FolderOpen,
  Gamepad2,
  Loader2,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  RefreshCw,
  Search,
  Ship,
} from "lucide-react";
import {
  api,
  type CardItem,
  type GameDTO,
  type GamesDTO,
  type LibraryDTO,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type Tab = "cg" | "scene" | "chapter";

type Status =
  | { state: "loading" }
  | { state: "setup" } // 需要 conf.Root
  | { state: "ready" }
  | { state: "error"; message: string };

const SIDEBAR_KEY = "ucgr.sidebarCollapsed";

// M2 事件网格（卡墙）。M3：卡片点击 → App 路由切回放页。
export default function EventGrid({
  onOpen,
}: {
  onOpen: (kind: string, id: string, title: string) => void;
}) {
  const [status, setStatus] = useState<Status>({ state: "loading" });
  const [lib, setLib] = useState<LibraryDTO | null>(null);
  const [games, setGames] = useState<GamesDTO | null>(null);
  const [tab, setTab] = useState<Tab>("cg");
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [busyGame, setBusyGame] = useState("");
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return window.localStorage.getItem(SIDEBAR_KEY) === "1";
    } catch {
      return false;
    }
  });
  const covers = useRef(new Map<string, string>()); // game:id → dataURL 缓存

  // 启动链：游戏登记表 → 当前游戏的库（有清单读清单，没有就扫描建库）。
  const boot = async () => {
    try {
      const gs = await api.listGames();
      setGames(gs);
      if (!gs.active) {
        setStatus({ state: "setup" });
        return;
      }
      await reload(false);
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    }
  };

  const reload = async (force = false) => {
    setStatus({ state: "loading" });
    try {
      const l = force ? await api.rescan() : await api.getLibrary();
      setLib(l);
      setStatus({ state: "ready" });
    } catch (e: any) {
      const msg = e?.message ?? String(e);
      setLib((prev) => prev ?? null);
      setStatus(
        msg.includes("还没有游戏库") || msg.includes("未设置游戏根目录")
          ? { state: "setup" }
          : { state: "error", message: msg },
      );
    }
  };

  useEffect(() => {
    void boot();
  }, []);

  // 左侧游戏栏折叠状态本地记忆；Ctrl+B 与 VS Code 一致。
  useEffect(() => {
    try {
      window.localStorage.setItem(SIDEBAR_KEY, collapsed ? "1" : "0");
    } catch {
      /* 无持久化不影响使用 */
    }
  }, [collapsed]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "b") {
        e.preventDefault();
        setCollapsed((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // 添加游戏（同一目录重复添加 = 切换并重扫，后端保证不重复登记）。
  const pickAndAdd = async () => {
    try {
      const dir = await api.pickDirectory();
      if (!dir) return;
      setBusy(true);
      setStatus({ state: "loading" });
      const l = await api.addGame(dir);
      const gs = await api.listGames();
      setGames(gs);
      covers.current.clear();
      setLib(l);
      setTab("cg");
      setStatus({ state: "ready" });
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    } finally {
      setBusy(false);
    }
  };

  const switchGame = async (id: string) => {
    if (!id || id === games?.active || busy) return;
    setBusy(true);
    setBusyGame(id);
    try {
      const l = await api.setActiveGame(id);
      const gs = await api.listGames();
      setGames(gs);
      covers.current.clear();
      setLib(l);
      setTab("cg");
      setStatus({ state: "ready" });
    } catch (e: any) {
      setStatus({ state: "error", message: e?.message ?? String(e) });
    } finally {
      setBusy(false);
      setBusyGame("");
    }
  };

  const cards: CardItem[] = useMemo(() => {
    if (!lib) return [];
    const query = q.trim().toLowerCase();
    const hit = (s: string) => !query || s.toLowerCase().includes(query);
    if (tab === "cg") {
      return (lib.cgs ?? [])
        .filter((c) => hit(c.title) || hit(c.id))
        .map((c) => ({
          kind: "cg" as const,
          id: c.id,
          title: c.title,
          // 旧库清单 cgreplay-lib.json 里 pages 可能为 null（CLI 旧格式），防御
          note: (c.pages ?? []).length > 1 ? `×${(c.pages ?? []).length}页` : "",
          ok: true,
        }));
    }
    if (tab === "scene") {
      return (lib.scenes ?? [])
        .filter((s) => hit(s.title))
        .map((s) => ({ kind: "scene" as const, id: s.id, title: s.title, note: "", ok: true as const }));
    }
    return (lib.chapters ?? [])
      .filter((ch) => hit(ch.title) || hit(ch.scene))
      .map((ch) => ({
        kind: "chapter" as const,
        id: ch.id,
        title: ch.title,
        note: `#${ch.index}`,
        ok: true,
      }));
  }, [lib, tab, q]);

  const coverOf = async (kind: string, id: string): Promise<string | null> => {
    const key = `${games?.active ?? lib?.root ?? ""}:${kind}:${id}`;
    const hit = covers.current.get(key);
    if (hit) return hit;
    try {
      const url = await api.getCover(kind, id);
      covers.current.set(key, url);
      return url;
    } catch {
      // 封面缺失不阻断卡片（回放时缺素材才红横幅，FR-12 主线不变）
      return null;
    }
  };

  if (status.state === "setup") {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="flex w-[460px] flex-col items-center gap-4 rounded-lg border border-border bg-card px-10 py-10 text-center">
          <FolderOpen className="h-9 w-9 text-primary" />
          <h1 className="text-lg font-semibold">先选游戏解包目录</h1>
          <p className="text-sm leading-relaxed text-muted-foreground">
            选「包含 raw\data\scenario 的那一层」。选择后会自动建立事件库。
            之后可随时从左侧游戏栏底部的「添加游戏」登记更多游戏，配置与 CLI 版
            cgreplay.exe 共用同一份 cgreplay.conf。
          </p>
          <Button onClick={pickAndAdd} disabled={busy} className="mt-2">
            {busy ? "建立中…" : "选择目录"}
          </Button>
        </div>
      </div>
    );
  }

  if (status.state === "loading" || (busy && !lib)) {
    return (
      <div className="flex min-h-screen items-center justify-center text-muted-foreground">
        正在建立事件库…
      </div>
    );
  }

  if (status.state === "error" && !lib) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="w-[520px] rounded-lg border border-red-900 bg-card px-8 py-7 text-center">
          <h1 className="text-base font-semibold text-red-400">出错了</h1>
          <p className="mt-3 break-all text-sm text-muted-foreground">{status.message}</p>
          <div className="mt-5 flex justify-center gap-3">
            <Button size="sm" variant="outline" onClick={pickAndAdd}>换个目录</Button>
            <Button size="sm" variant="outline" onClick={() => void reload(true)}>重试</Button>
          </div>
        </div>
      </div>
    );
  }

  const banner =
    status.state === "error" && lib ? status.message : null;
  const gameList = games?.games ?? [];
  const activeGame = gameList.find((g) => g.id === games?.active);
  const activeTitle = activeGame?.title ?? lib?.game ?? "CG 演出回放";

  return (
    <div className="flex h-screen overflow-hidden">
      <GameSidebar
        games={gameList}
        active={games?.active ?? ""}
        busy={busy}
        busyGame={busyGame}
        collapsed={collapsed}
        onToggle={() => setCollapsed((v) => !v)}
        onSelect={(id) => void switchGame(id)}
        onAdd={() => void pickAndAdd()}
      />

      <div className="flex min-w-0 flex-1 flex-col">
        {/* 顶栏：当前游戏 + 页签 + 搜索 + 刷新 */}
        <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background/95 px-4 backdrop-blur lg:gap-3">
          <div className="hidden min-w-0 max-w-[180px] shrink-0 lg:block">
            <div className="flex items-center gap-2">
              <h1 className="truncate text-sm font-semibold">{activeTitle}</h1>
              {gameList.length > 1 && (
                <Badge variant="secondary" className="shrink-0 px-1.5 text-[10px]">
                  {gameList.length} 库
                </Badge>
              )}
            </div>
            <p className="truncate text-[10px] leading-tight text-muted-foreground" title={lib?.root}>
              {lib?.root ?? "从左侧选择游戏库"}
            </p>
          </div>
          <nav className="flex shrink-0 gap-1">
            {(
              [
                ["cg", `CG鉴赏 ${lib?.cgs?.length ?? 0}`],
                ["scene", `主线场景 ${lib?.scenes?.length ?? 0}`],
                ["chapter", `演出段落 ${lib?.chapters?.length ?? 0}`],
              ] as [Tab, string][]
            ).map(([k, label]) => (
              <button
                key={k}
                onClick={() => setTab(k)}
                className={
                  "rounded-md px-2.5 py-1.5 text-sm transition-colors " +
                  (tab === k
                    ? "bg-secondary font-medium text-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground")
                }
              >
                {label}
              </button>
            ))}
          </nav>
          <div className="flex-1" />
          <div className="relative shrink-0">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="搜索 cg17 / 标题…"
              className="h-9 w-32 rounded-md border border-input bg-input/40 pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring lg:w-44 xl:w-60"
            />
          </div>
          <Button
            variant="outline"
            size="icon"
            className="shrink-0"
            title="重扫当前游戏"
            onClick={() => void reload(true)}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
        </header>

        {/* 错误横幅区：任何异常都常驻可见（对齐 FR-12） */}
        {banner && (
          <div className="shrink-0 bg-destructive px-5 py-1.5 text-sm">{banner}</div>
        )}

        {/* 卡片墙 */}
        <main className="grid flex-1 auto-rows-max content-start gap-4 overflow-y-auto p-5"
          style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
          {cards.map((c) => (
            <EventCard
              key={`${lib?.root ?? ""}:${c.kind}:${c.id}`}
              card={c}
              lookup={coverOf}
              onClick={() => onOpen(c.kind, c.id, c.title)}
            />
          ))}
          {cards.length === 0 && (
            <div className="col-span-full py-24 text-center text-sm text-muted-foreground">
              没有匹配的事件
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

// ---------- 左侧游戏栏（VS Code 侧栏风格） ----------

function GameSidebar({
  games,
  active,
  busy,
  busyGame,
  collapsed,
  onToggle,
  onSelect,
  onAdd,
}: {
  games: GameDTO[];
  active: string;
  busy: boolean;
  busyGame: string;
  collapsed: boolean;
  onToggle: () => void;
  onSelect: (id: string) => void;
  onAdd: () => void;
}) {
  const [filter, setFilter] = useState("");
  const q = filter.trim().toLowerCase();
  const shown = games.filter(
    (g) =>
      !q ||
      g.title.toLowerCase().includes(q) ||
      g.root.toLowerCase().includes(q) ||
      g.id.toLowerCase().includes(q),
  );

  if (collapsed) {
    return (
      <aside className="flex h-full w-16 shrink-0 flex-col border-r border-border bg-card/30">
        <div className="flex h-14 shrink-0 items-center justify-center border-b border-border">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Ship className="h-4 w-4" />
          </div>
        </div>
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto px-2 py-3">
          {games.map((g) => {
            const isActive = g.id === active;
            const pending = busyGame === g.id;
            return (
              <button
                key={g.id}
                onClick={() => onSelect(g.id)}
                disabled={busy}
                aria-current={isActive ? "page" : undefined}
                title={`${g.title}\n${g.root}`}
                className={cn(
                  "mx-auto flex h-9 w-9 items-center justify-center rounded-md border text-[11px] font-semibold transition-colors",
                  isActive
                    ? "border-primary/50 bg-primary/15 text-primary"
                    : "border-transparent text-muted-foreground hover:bg-accent hover:text-foreground",
                  busy && "cursor-not-allowed opacity-50",
                )}
              >
                {pending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : gameInitials(g.title)}
              </button>
            );
          })}
        </div>
        <div className="space-y-1 border-t border-border p-2">
          <button
            onClick={onAdd}
            disabled={busy}
            title="添加游戏"
            className="mx-auto flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Plus className="h-4 w-4" />
          </button>
          <button
            onClick={onToggle}
            title="展开游戏栏 (Ctrl+B)"
            className="mx-auto flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <PanelLeftOpen className="h-4 w-4" />
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className="flex h-full w-[240px] shrink-0 flex-col border-r border-border bg-card/30">
      <div className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary">
          <Ship className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1 leading-tight">
          <div className="truncate text-[13px] font-semibold">CG 演出回放</div>
          <div className="truncate text-[10px] text-muted-foreground">多游戏库</div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0 text-muted-foreground"
          title="收起游戏栏 (Ctrl+B)"
          onClick={onToggle}
        >
          <PanelLeftClose className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex items-center justify-between px-3 pb-1.5 pt-4">
        <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          游戏库
        </span>
        <Badge variant="secondary" className="px-1.5 text-[10px]">
          {games.length}
        </Badge>
      </div>

      {games.length > 4 && (
        <div className="px-2 pb-2">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="筛选游戏…"
              className="h-8 w-full rounded-md border border-input bg-input/40 pl-7 pr-2 text-xs outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>
      )}

      <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
        {shown.map((g) => {
          const isActive = g.id === active;
          const pending = busyGame === g.id;
          return (
            <button
              key={g.id}
              onClick={() => onSelect(g.id)}
              disabled={busy}
              aria-current={isActive ? "page" : undefined}
              title={`${g.title}\n${g.root}`}
              className={cn(
                "group relative flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left transition-colors",
                isActive
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                busy && "cursor-not-allowed opacity-60",
              )}
            >
              {isActive && (
                <span className="absolute left-0 top-1/2 h-5 w-0.5 -translate-y-1/2 rounded-full bg-primary" />
              )}
              <span
                className={cn(
                  "flex h-8 w-8 shrink-0 items-center justify-center rounded-md border",
                  isActive
                    ? "border-primary/40 bg-primary/15 text-primary"
                    : "border-border bg-background/40",
                )}
              >
                <Gamepad2 className="h-4 w-4" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[13px] font-medium">{g.title}</span>
                <span className="block truncate text-[10px] text-muted-foreground">
                  {shortRoot(g.root)}
                </span>
              </span>
              {pending ? (
                <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
              ) : isActive ? (
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
              ) : null}
            </button>
          );
        })}
        {shown.length === 0 && (
          <div className="px-2 py-6 text-center text-xs text-muted-foreground">
            没有匹配的游戏
          </div>
        )}
      </div>

      <div className="border-t border-border p-2">
        <Button
          variant="ghost"
          className="h-9 w-full justify-start gap-2 px-2 text-xs text-muted-foreground hover:text-foreground"
          onClick={onAdd}
          disabled={busy}
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
          {busy ? "正在登记…" : "添加游戏"}
        </Button>
      </div>
    </aside>
  );
}

/** 折叠态图标：中文取首字，拉丁字母取前两位。 */
function gameInitials(title: string): string {
  const t = (title ?? "").trim();
  if (!t) return "?";
  const chars = Array.from(t);
  return /^[A-Za-z0-9]/.test(chars[0]) ? chars.slice(0, 2).join("").toUpperCase() : chars[0];
}

/** 侧栏副标题只显示盘符 + 父目录；标题本身通常就是游戏目录名，完整路径放 title。 */
function shortRoot(root: string): string {
  const parts = root.split(/[\\/]+/).filter(Boolean);
  if (parts.length === 0) return root;
  const tail = parts[parts.length - 1];
  let parent = parts[parts.length - 2];
  if (parent && /^[0-9a-f]{12,}$/i.test(parent) && parts.length >= 3) {
    parent = parts[parts.length - 3];
  }
  const drive = /^[A-Za-z]:$/.test(parts[0]) ? `${parts[0]}\\…` : "…";
  return parent ? `${drive}\\${parent}` : `${drive}\\${tail}`;
}

// ---------- 卡片 ----------
function EventCard({
  card,
  lookup,
  onClick,
}: {
  card: CardItem;
  lookup: (kind: string, id: string) => Promise<string | null>;
  onClick: () => void;
}) {
  const [cover, setCover] = useState<string | null>(null);
  const [miss, setMiss] = useState(false); // 封面缺失：角标可见，不静默
  useEffect(() => {
    let alive = true;
    void lookup(card.kind, card.id).then((u) => {
      if (!alive) return;
      setCover(u);
      if (!u) setMiss(true);
    });
    return () => {
      alive = false;
    };
  }, [card.kind, card.id]);

  return (
    <button
      onClick={onClick}
      className="group relative overflow-hidden rounded-lg border border-border bg-card text-left transition-shadow hover:border-primary/60 hover:shadow-md"
    >
      <div className="relative aspect-[220/124] w-full overflow-hidden bg-secondary/60">
        {cover ? (
          <img src={cover} alt="" className="h-full w-full object-cover" draggable={false} />
        ) : (
          <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-secondary to-accent/40 text-muted-foreground/40">
            <Ship className="h-8 w-8" />
          </div>
        )}
        {miss && (
          <span className="absolute left-1.5 top-1.5 rounded border border-amber-900 bg-amber-950/80 px-1 py-0.5 text-[10px] text-amber-400">
            无封面
          </span>
        )}
      </div>
      <div className="space-y-1 px-3 py-2">
        <div className="truncate text-xs text-muted-foreground">{card.id}</div>
        <div className="truncate text-sm">{card.title}</div>
        {card.note && (
          <Badge variant="secondary" className="mt-0.5">
            {card.note}
          </Badge>
        )}
      </div>
    </button>
  );
}
