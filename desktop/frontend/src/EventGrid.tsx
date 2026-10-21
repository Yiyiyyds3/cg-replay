import { useEffect, useMemo, useRef, useState } from "react";
import {
  Clock,
  Database,
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
  type CatalogEventDTO,
  type CatalogRecentDTO,
  type CatalogStatsDTO,
  type CatalogTextHitDTO,
  type GameDTO,
  type GamesDTO,
  type LibraryDTO,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type Tab = "all" | "cg" | "scene" | "chapter";

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
  const [catalog, setCatalog] = useState<CatalogStatsDTO | null>(null);
  const [tab, setTab] = useState<Tab>("cg");
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [busyGame, setBusyGame] = useState("");
  // ---------- 「全部」页签：跨游戏卡片墙 / 台词检索 / 最近播放 ----------
  const [allEvents, setAllEvents] = useState<CatalogEventDTO[] | null>(null);
  const [allWarnings, setAllWarnings] = useState<string[]>([]);
  const [textHits, setTextHits] = useState<CatalogTextHitDTO[] | null>(null);
  const [textIndexed, setTextIndexed] = useState(true);
  const [recentItems, setRecentItems] = useState<CatalogRecentDTO[]>([]);
  const [textCounts, setTextCounts] = useState<Record<string, number>>({});
  const [indexing, setIndexing] = useState<{
    title: string;
    done: number;
    total: number;
  } | null>(null);
  const allBooted = useRef(false); // StrictMode 双挂载只加载一次
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
      void refreshCatalog();
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

  // 中心库只读统计；失败时 Error 落到角标，不阻断回放。
  const refreshCatalog = async () => {
    try {
      setCatalog(await api.catalogStats());
    } catch (e: any) {
      setCatalog({
        games: 0,
        events: 0,
        text_lines: 0,
        error: e?.message ?? String(e),
      });
    }
  };

  // ---------- 「全部」页签数据 ----------

  // 串行索引一批游戏的台词（CatalogIndexText 幂等；单游戏失败不阻断其余）。
  const indexGames = async (list: GameDTO[]) => {
    for (let i = 0; i < list.length; i++) {
      const g = list[i];
      setIndexing({ title: g.title, done: i, total: list.length });
      try {
        await api.catalogIndexText(g.id);
      } catch (e: any) {
        setAllWarnings((w) => [
          ...w,
          `索引「${g.title}」台词失败：${e?.message ?? String(e)}`,
        ]);
      }
    }
    setIndexing(null);
    try {
      const [tc, st] = await Promise.all([api.catalogTextCounts(), api.catalogStats()]);
      setTextCounts(tc);
      setCatalog(st);
    } catch {
      /* 角标刷新失败不影响已写入的索引 */
    }
  };

  // 进入「全部」：查询前由后端逐游戏同步 catalog；0 台词游戏按需抽台词。
  const loadAll = async () => {
    try {
      const gs = await api.listGames();
      setGames(gs);
      const [ev, rc, tc] = await Promise.all([
        api.catalogEvents("", 2000),
        api.catalogRecent(12),
        api.catalogTextCounts(),
      ]);
      setAllEvents(ev.events);
      setAllWarnings(ev.warnings);
      setRecentItems(rc);
      setTextCounts(tc);
      const pending = gs.games.filter((g) => !tc[g.id]);
      if (pending.length > 0) await indexGames(pending);
    } catch (e: any) {
      setAllWarnings([e?.message ?? String(e)]);
    }
  };

  // 切到「全部」时加载一次（StrictMode 双挂载用 ref 防重入）。
  useEffect(() => {
    if (tab !== "all" || allBooted.current) return;
    allBooted.current = true;
    void loadAll();
  }, [tab]);

  // 台词检索防抖（仅「全部」页签；空查询回到分组卡片墙）。
  useEffect(() => {
    if (tab !== "all") return;
    const query = q.trim();
    if (!query) {
      setTextHits(null);
      return;
    }
    const h = setTimeout(() => {
      void api
        .catalogSearchText(query, 100)
        .then((r) => {
          setTextHits(r.hits);
          setTextIndexed(r.indexed);
          if (r.warnings.length) setAllWarnings(r.warnings);
        })
        .catch((e: any) => setAllWarnings([e?.message ?? String(e)]));
    }, 250);
    return () => clearTimeout(h);
  }, [tab, q]);

  // 跨游戏打开：先切到该游戏（后端 SetActiveGame 重扫/读库），再进回放。
  const openFromGame = async (
    gameID: string,
    kind: string,
    id: string,
    title: string,
  ) => {
    if (gameID && gameID !== games?.active) {
      setBusy(true);
      try {
        const l = await api.setActiveGame(gameID);
        const gs = await api.listGames();
        setGames(gs);
        setLib(l);
        covers.current.clear();
      } catch (e: any) {
        setStatus({ state: "error", message: e?.message ?? String(e) });
        return;
      } finally {
        setBusy(false);
      }
    }
    onOpen(kind, id, title);
  };

  // 封面按游戏路由（跨游戏卡片墙）；缺失回 null，不阻断卡片。
  const coverFor = async (
    gameID: string,
    kind: string,
    id: string,
  ): Promise<string | null> => {
    const key = `${gameID}:${kind}:${id}`;
    const hit = covers.current.get(key);
    if (hit) return hit;
    try {
      const url = await api.getCoverForGame(gameID, kind, id);
      covers.current.set(key, url);
      return url;
    } catch {
      return null;
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
      void refreshCatalog();
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
      void refreshCatalog();
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
        catalog={catalog}
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
                ["all", `全部 ${allEvents ? allEvents.length : "·"}`],
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
              placeholder={tab === "all" ? "搜索全部游戏：标题 / 台词…" : "搜索 cg17 / 标题…"}
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

        {/* 「全部」页签：跨游戏卡片墙 / 台词检索 / 最近播放 */}
        {tab === "all" ? (
          <AllGames
            events={allEvents ?? []}
            loaded={allEvents !== null}
            warnings={allWarnings}
            indexing={indexing}
            recent={recentItems}
            textHits={textHits}
            textIndexed={textIndexed}
            textCounts={textCounts}
            busy={busy}
            onOpen={openFromGame}
            onCover={coverFor}
            onIndexAll={() => {
              const pending = gameList.filter((g) => !textCounts[g.id]);
              if (pending.length > 0) void indexGames(pending);
              else void loadAll();
            }}
          />
        ) : (
          /* 卡片墙 */
          <main className="grid flex-1 auto-rows-max content-start gap-4 overflow-y-auto p-5"
            style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
            {cards.map((c) => (
              <EventCard
                key={`${lib?.root ?? ""}:${c.kind}:${c.id}`}
                card={c}
                gameID={games?.active ?? ""}
                lookup={coverFor}
                onClick={() => onOpen(c.kind, c.id, c.title)}
              />
            ))}
            {cards.length === 0 && (
              <div className="col-span-full py-24 text-center text-sm text-muted-foreground">
                没有匹配的事件
              </div>
            )}
          </main>
        )}
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
  catalog,
  onToggle,
  onSelect,
  onAdd,
}: {
  games: GameDTO[];
  active: string;
  busy: boolean;
  busyGame: string;
  collapsed: boolean;
  catalog: CatalogStatsDTO | null;
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
          <div
            title={catalogTitle(catalog)}
            className="mx-auto flex h-8 w-9 items-center justify-center"
          >
            <Database
              className={cn(
                "h-4 w-4",
                catalog?.error ? "text-amber-400" : "text-muted-foreground/70",
              )}
            />
          </div>
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

      <div className="border-t border-border px-3 py-2" title={catalogTitle(catalog)}>
        <div className="flex items-center gap-1.5 text-[10px]">
          <Database
            className={cn(
              "h-3.5 w-3.5 shrink-0",
              catalog?.error ? "text-amber-400" : "text-primary/70",
            )}
          />
          {catalog?.error ? (
            <span className="truncate text-amber-400">索引失败</span>
          ) : catalog ? (
            <span className="truncate text-muted-foreground">
              索引 {catalog.events} 事件 · {catalog.games} 库
            </span>
          ) : (
            <span className="truncate text-muted-foreground">索引检查中…</span>
          )}
        </div>
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

/** 索引角标 tooltip：失败原因常驻可查，不静默。 */
function catalogTitle(c: CatalogStatsDTO | null): string {
  if (!c) return "中心库索引检查中…";
  if (c.error) return `中心库索引失败：${c.error}`;
  return `本地 SQLite 索引\n${c.games} 个游戏 / ${c.events} 个事件 / ${c.text_lines} 条台词\n只存可检索元数据，回放仍读原始素材。`;
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

// ---------- 「全部」页签：跨游戏卡片墙 / 台词检索 / 最近播放 ----------

const KIND_LABEL: Record<string, string> = { cg: "CG", scene: "场景", chapter: "段落" };

function kindLabel(kind: string): string {
  return KIND_LABEL[kind] ?? kind;
}

/** 毫秒 → m:ss（台词行的时刻参考）。 */
function mmss(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/** catalog 事件 → 卡片视图（kind 收窄成 CardItem 联合，未知 kind 走 ok:false）。 */
function cardFor(e: CatalogEventDTO): CardItem {
  switch (e.kind) {
    case "cg":
      return { kind: "cg", id: e.id, title: e.title, note: e.pages > 1 ? `×${e.pages}页` : "", ok: true };
    case "scene":
      return { kind: "scene", id: e.id, title: e.title, note: "", ok: true };
    case "chapter":
      return { kind: "chapter", id: e.id, title: e.title, note: "", ok: true };
    default:
      return { kind: e.kind, id: e.id, title: e.title, note: "", ok: false };
  }
}

function AllGames({
  events,
  loaded,
  warnings,
  indexing,
  recent,
  textHits,
  textIndexed,
  textCounts,
  busy,
  onOpen,
  onCover,
  onIndexAll,
}: {
  events: CatalogEventDTO[];
  loaded: boolean;
  warnings: string[];
  indexing: { title: string; done: number; total: number } | null;
  recent: CatalogRecentDTO[];
  textHits: CatalogTextHitDTO[] | null;
  textIndexed: boolean;
  textCounts: Record<string, number>;
  busy: boolean;
  onOpen: (gameID: string, kind: string, id: string, title: string) => void;
  onCover: (gameID: string, kind: string, id: string) => Promise<string | null>;
  onIndexAll: () => void;
}) {
  // 按游戏分组（后端按游戏标题排序，保持出现序即分组序）。
  const groups = useMemo(() => {
    const out: { game: CatalogEventDTO; items: CatalogEventDTO[] }[] = [];
    const idx = new Map<string, number>();
    for (const e of events) {
      let gi = idx.get(e.game_id);
      if (gi === undefined) {
        gi = out.length;
        idx.set(e.game_id, gi);
        out.push({ game: e, items: [] });
      }
      out[gi].items.push(e);
    }
    return out;
  }, [events]);

  // 台词行的事件标题：从已加载事件里查（查不到回落 event_id）。
  const titleOf = (gameID: string, kind: string, id: string) =>
    events.find((e) => e.game_id === gameID && e.kind === kind && e.id === id)?.title ?? id;

  if (!loaded) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        正在汇总全部游戏…
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* 同步/索引告警：部分成功也明示（FR-12 不静默） */}
      {(warnings.length > 0 || indexing) && (
        <div className="shrink-0 space-y-1 border-b border-amber-900/60 bg-amber-950/40 px-5 py-2 text-xs text-amber-300">
          {indexing && (
            <div className="flex items-center gap-2">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              正在抽取台词：{indexing.title}（{indexing.done + 1}/{indexing.total}）…
            </div>
          )}
          {warnings.map((w, i) => (
            <div key={i} className="break-all">
              {w}
            </div>
          ))}
        </div>
      )}

      <main className="flex-1 space-y-6 overflow-y-auto p-5">
        {/* 最近播放 */}
        {recent.length > 0 && !textHits && (
          <section className="space-y-2">
            <h2 className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              <Clock className="h-3.5 w-3.5" />
              最近播放
            </h2>
            <div className="flex gap-2 overflow-x-auto pb-1">
              {recent.map((r) => (
                <button
                  key={`${r.game_id}:${r.kind}:${r.event_id}`}
                  disabled={busy}
                  onClick={() => onOpen(r.game_id, r.kind, r.event_id, r.title)}
                  className="shrink-0 rounded-md border border-border bg-card px-3 py-1.5 text-left text-xs transition-colors hover:border-primary/60 disabled:opacity-50"
                  title={`${r.game_title}\n${r.title}`}
                >
                  <span className="block max-w-[180px] truncate font-medium">{r.title}</span>
                  <span className="block max-w-[180px] truncate text-muted-foreground">
                    {r.game_title} · ×{r.play_count}
                  </span>
                </button>
              ))}
            </div>
          </section>
        )}

        {/* 台词检索结果 */}
        {textHits !== null ? (
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              台词命中 {textHits.length} 条
            </h2>
            {!textIndexed && (
              <div className="flex items-center gap-3 rounded-md border border-amber-900/60 bg-amber-950/40 px-4 py-2.5 text-xs text-amber-300">
                <span className="flex-1">
                  台词索引尚未抽取（当前只匹配事件标题）。抽取后可按原文检索全部台词。
                </span>
                <Button size="sm" variant="outline" onClick={onIndexAll} disabled={busy}>
                  立即索引
                </Button>
              </div>
            )}
            {textHits.length === 0 ? (
              <div className="py-16 text-center text-sm text-muted-foreground">没有匹配的台词</div>
            ) : (
              <ul className="divide-y divide-border rounded-md border border-border">
                {textHits.map((h) => (
                  <li key={h.line_id}>
                    <button
                      disabled={busy}
                      onClick={() =>
                        onOpen(h.game_id, h.kind, h.event_id, titleOf(h.game_id, h.kind, h.event_id))
                      }
                      className="flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors hover:bg-accent/60 disabled:opacity-50"
                    >
                      <span
                        className="w-28 shrink-0 truncate text-xs text-muted-foreground"
                        title={h.game_title}
                      >
                        {h.game_title}
                      </span>
                      <Badge variant="secondary" className="shrink-0 px-1.5 text-[10px]">
                        {kindLabel(h.kind)}
                      </Badge>
                      <span className="w-20 shrink-0 truncate text-xs text-muted-foreground">
                        {h.speaker || "旁白"}
                      </span>
                      <span className="min-w-0 flex-1 truncate text-sm">{h.text}</span>
                      <span className="shrink-0 text-[10px] text-muted-foreground">{mmss(h.t_ms)}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        ) : groups.length === 0 ? (
          <div className="py-24 text-center text-sm text-muted-foreground">
            还没有任何事件。从左侧游戏栏底部「添加游戏」登记解包目录。
          </div>
        ) : (
          groups.map((g) => {
            const cgN = g.items.filter((e) => e.kind === "cg").length;
            const scN = g.items.filter((e) => e.kind === "scene").length;
            const chN = g.items.filter((e) => e.kind === "chapter").length;
            const lines = textCounts[g.game.game_id] ?? 0;
            return (
              <section key={g.game.game_id} className="space-y-3">
                <div className="flex items-baseline gap-2">
                  <h2 className="text-sm font-semibold">{g.game.game_title}</h2>
                  <span
                    className="truncate text-[10px] text-muted-foreground"
                    title={g.game.game_root}
                  >
                    {shortRoot(g.game.game_root)}
                  </span>
                  <span className="flex-1" />
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    {cgN > 0 && `CG ${cgN} · `}
                    {scN > 0 && `场景 ${scN} · `}
                    {chN > 0 && `段落 ${chN} · `}
                    台词 {lines}
                  </span>
                </div>
                <div
                  className="grid auto-rows-max gap-4"
                  style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}
                >
                  {g.items.map((e) => (
                    <EventCard
                      key={`${e.game_id}:${e.kind}:${e.id}`}
                      card={cardFor(e)}
                      gameID={e.game_id}
                      lookup={onCover}
                      onClick={() => onOpen(e.game_id, e.kind, e.id, e.title)}
                    />
                  ))}
                </div>
              </section>
            );
          })
        )}
      </main>
    </div>
  );
}

// ---------- 卡片 ----------
function EventCard({
  card,
  gameID,
  lookup,
  onClick,
}: {
  card: CardItem;
  gameID: string;
  lookup: (gameID: string, kind: string, id: string) => Promise<string | null>;
  onClick: () => void;
}) {
  const [cover, setCover] = useState<string | null>(null);
  const [miss, setMiss] = useState(false); // 封面缺失：角标可见，不静默
  useEffect(() => {
    let alive = true;
    void lookup(gameID, card.kind, card.id).then((u) => {
      if (!alive) return;
      setCover(u);
      if (!u) setMiss(true);
    });
    return () => {
      alive = false;
    };
  }, [gameID, card.kind, card.id]);

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
