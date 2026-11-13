// VSCode 风格工作区外壳：标题栏 / 活动栏 / 标签栏 / 状态栏。
// 纯展示组件：数据与交互由 App / EventGrid 注入，避免 shell 反向依赖业务层。

import { useEffect, type ReactNode } from "react";
import {
  Clapperboard,
  FileText,
  History,
  Image as ImageIcon,
  Layers3,
  LibraryBig,
  ListTree,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Ship,
  Star,
  Users,
} from "lucide-react";
import { cn } from "@/lib/utils";

export type ActivityId = "library" | "recent" | "favorites" | "characters" | "settings";

export interface ActivityCounts {
  library?: number;
  recent?: number;
  favorites?: number;
  characters?: number;
  settings?: number;
}

export interface WorkspaceTab {
  id: string;
  title: string;
  kind: "library" | "replay";
  /** CG / 场景 / 段落；用作标签右侧的弱化说明。 */
  kindLabel?: string;
  /** 游戏名，hover 时展示完整来源。 */
  gameTitle?: string;
  closable?: boolean;
}

export interface ShellStatus {
  connected: boolean;
  bridge: string;
  libraryName: string;
  eventCount: number;
  activeTitle: string;
}

const ACTIVITIES: {
  id: ActivityId;
  label: string;
  shortcut: string;
  icon: typeof LibraryBig;
}[] = [
  { id: "library", label: "资源库", shortcut: "事件与游戏", icon: LibraryBig },
  { id: "recent", label: "最近播放", shortcut: "最近事件", icon: History },
  { id: "favorites", label: "收藏", shortcut: "事件与角色", icon: Star },
  { id: "characters", label: "角色", shortcut: "台词聚合", icon: Users },
];

const TAB_ICONS = {
  library: LibraryBig,
  cg: ImageIcon,
  scene: Clapperboard,
  chapter: ListTree,
  replay: FileText,
} as const;

function tabIcon(tab: WorkspaceTab) {
  if (tab.kind === "library") return TAB_ICONS.library;
  const key = tab.kindLabel?.toLowerCase();
  if (key === "cg") return TAB_ICONS.cg;
  if (key === "scene") return TAB_ICONS.scene;
  if (key === "chapter") return TAB_ICONS.chapter;
  return TAB_ICONS.replay;
}

export function TitleBar({
  libraryName,
  libraryRoot,
  connected,
  bridge,
  sidebarCollapsed,
  onToggleSidebar,
}: {
  libraryName: string;
  libraryRoot?: string;
  connected: boolean;
  bridge: string;
  sidebarCollapsed: boolean;
  onToggleSidebar: () => void;
}) {
  return (
    <header className="titlebar">
      <div className="titlebar-drag">
        <span className="titlebar-logo" aria-hidden>
          <Ship className="h-4 w-4" />
        </span>
        <span className="titlebar-app">CG Replay Library</span>
        <span className="titlebar-divider" aria-hidden />
        <span className="titlebar-library" title={libraryRoot}>
          {libraryName}
        </span>
        {libraryRoot && (
          <span className="titlebar-path" title={libraryRoot}>
            {libraryRoot}
          </span>
        )}
      </div>

      <div className="titlebar-center" title={bridge || "等待后端版本号"}>
        <span className="titlebar-command">
          <span className="titlebar-command-dot" />
          {connected ? "已连接到本地回放引擎" : "正在连接本地回放引擎…"}
        </span>
      </div>

      <div className="titlebar-actions">
        <span
          className={cn("titlebar-status", connected ? "is-online" : "is-offline")}
          title={bridge || "后端未响应"}
        >
          <span className="status-dot" />
          {connected ? "Bridge 在线" : "Bridge 离线"}
        </span>
        <button
          type="button"
          className="titlebar-icon-button"
          onClick={onToggleSidebar}
          title={sidebarCollapsed ? "展开侧边栏 (Ctrl+B)" : "收起侧边栏 (Ctrl+B)"}
          aria-label={sidebarCollapsed ? "展开侧边栏" : "收起侧边栏"}
        >
          {sidebarCollapsed ? (
            <PanelLeftOpen className="h-4 w-4" />
          ) : (
            <PanelLeftClose className="h-4 w-4" />
          )}
        </button>
      </div>
    </header>
  );
}

export function ActivityBar({
  active,
  counts,
  onSelect,
}: {
  active: ActivityId;
  counts: ActivityCounts;
  onSelect: (id: ActivityId) => void;
}) {
  return (
    <nav className="activitybar" aria-label="主导航">
      {ACTIVITIES.map(({ id, label, shortcut, icon: Icon }) => {
        const count = counts[id];
        const selected = active === id;
        return (
          <button
            key={id}
            type="button"
            className={cn("activitybar-item", selected && "is-active")}
            onClick={() => onSelect(id)}
            title={`${label} · ${shortcut}`}
            aria-label={label}
            aria-current={selected ? "page" : undefined}
          >
            <span className="activitybar-indicator" aria-hidden />
            <Icon className="activitybar-icon" />
            {typeof count === "number" && count > 0 && (
              <span className="activitybar-badge">{count > 99 ? "99+" : count}</span>
            )}
          </button>
        );
      })}

      <div className="activitybar-spacer" />
      <button
        type="button"
        className={cn("activitybar-item", active === "settings" && "is-active")}
        onClick={() => onSelect("settings")}
        title="设置 · 解包与索引"
        aria-label="设置"
        aria-current={active === "settings" ? "page" : undefined}
      >
        <span className="activitybar-indicator" aria-hidden />
        <Settings className="activitybar-icon" />
      </button>
    </nav>
  );
}

export function EditorTabs({
  tabs,
  activeId,
  onSelect,
  onClose,
}: {
  tabs: WorkspaceTab[];
  activeId: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
}) {
  return (
    <div className="editor-tabs" role="tablist" aria-label="已打开事件">
      <div className="editor-tabs-scroll">
        {tabs.map((tab) => {
          const Icon = tabIcon(tab);
          const active = tab.id === activeId;
          return (
            <div
              key={tab.id}
              role="tab"
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              className={cn("editor-tab", active && "is-active")}
              onClick={() => onSelect(tab.id)}
              onAuxClick={(e) => {
                if (e.button === 1 && tab.closable) {
                  e.preventDefault();
                  onClose(tab.id);
                }
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onSelect(tab.id);
                }
              }}
              title={[tab.title, tab.gameTitle, tab.kindLabel].filter(Boolean).join(" · ")}
            >
              <Icon className="editor-tab-icon" />
              <span className="editor-tab-title">{tab.title}</span>
              {tab.kindLabel && <span className="editor-tab-kind">{tab.kindLabel}</span>}
              {tab.closable && (
                <button
                  type="button"
                  className="editor-tab-close"
                  onClick={(e) => {
                    e.stopPropagation();
                    onClose(tab.id);
                  }}
                  title={`关闭 ${tab.title}`}
                  aria-label={`关闭 ${tab.title}`}
                >
                  <span aria-hidden>×</span>
                </button>
              )}
            </div>
          );
        })}
      </div>
      <div className="editor-tabs-spacer" />
      <span className="editor-tabs-hint" title="Ctrl+B 收起侧栏">
        <Layers3 className="h-3.5 w-3.5" />
        工作区
      </span>
    </div>
  );
}

export function StatusBar({ status }: { status: ShellStatus }) {
  return (
    <footer className="statusbar">
      <span className="statusbar-item" title={status.bridge || "等待后端版本号"}>
        <span className={cn("statusbar-dot", status.connected ? "is-online" : "is-offline")} />
        {status.connected ? "后端已连接" : "后端连接中"}
      </span>
      <span className="statusbar-item" title={status.libraryName}>
        <LibraryBig className="h-3 w-3" />
        {status.libraryName || "未选择游戏库"}
      </span>
      <span className="statusbar-item">
        <ListTree className="h-3 w-3" />
        {status.eventCount} 个事件
      </span>
      <span className="statusbar-spacer" />
      <span className="statusbar-item statusbar-muted" title="当前编辑器">
        {status.activeTitle}
      </span>
      <span className="statusbar-item statusbar-muted">UTF-8</span>
      <span className="statusbar-item statusbar-muted">
        {status.bridge || "CG Replay"} · VSCode Workspace
      </span>
    </footer>
  );
}

export function WorkspaceShell({
  libraryName,
  libraryRoot,
  connected,
  bridge,
  sidebarCollapsed,
  onToggleSidebar,
  activity,
  activityCounts,
  onSelectActivity,
  sidebar,
  tabs,
  activeTabId,
  onSelectTab,
  onCloseTab,
  status,
  children,
}: {
  libraryName: string;
  libraryRoot?: string;
  connected: boolean;
  bridge: string;
  sidebarCollapsed: boolean;
  onToggleSidebar: () => void;
  activity: ActivityId;
  activityCounts: ActivityCounts;
  onSelectActivity: (id: ActivityId) => void;
  sidebar: ReactNode;
  tabs: WorkspaceTab[];
  activeTabId: string;
  onSelectTab: (id: string) => void;
  onCloseTab: (id: string) => void;
  status: ShellStatus;
  children: ReactNode;
}) {
  // VSCode 同款 Ctrl+B：收起/展开侧栏。
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "b") {
        e.preventDefault();
        onToggleSidebar();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onToggleSidebar]);

  return (
    <div className="app-shell">
      <TitleBar
        libraryName={libraryName}
        libraryRoot={libraryRoot}
        connected={connected}
        bridge={bridge}
        sidebarCollapsed={sidebarCollapsed}
        onToggleSidebar={onToggleSidebar}
      />

      <div className="workspace-body">
        <ActivityBar
          active={activity}
          counts={activityCounts}
          onSelect={onSelectActivity}
        />

        {!sidebarCollapsed && <div className="sidebar-region">{sidebar}</div>}

        <section className="editor-region" aria-label="编辑器">
          <EditorTabs
            tabs={tabs}
            activeId={activeTabId}
            onSelect={onSelectTab}
            onClose={onCloseTab}
          />
          <div className="editor-surface">{children}</div>
        </section>
      </div>

      <StatusBar status={status} />
    </div>
  );
}
