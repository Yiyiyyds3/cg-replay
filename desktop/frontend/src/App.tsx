// 根组件：VSCode 风格工作区。
// 仓库标签常驻；事件回放作为可关闭的编辑器标签，后台会话切换时
// 记录当前播放位置，重新激活标签后 seek 回去，避免多标签丢进度。

import { useEffect, useRef, useState } from "react";
import EventGrid from "@/EventGrid";
import Replay from "@/Replay";
import { api, type SessionMeta } from "@/lib/api";
import type { WorkspaceTab } from "@/shell/WorkspaceShell";

const LIBRARY_TAB_ID = "library";

type OpenTab = WorkspaceTab & {
  meta?: SessionMeta;
  position?: number;
};

const LIBRARY_TAB: OpenTab = {
  id: LIBRARY_TAB_ID,
  kind: "library",
  title: "事件库",
  kindLabel: "资源库",
  closable: false,
};

export default function App() {
  const [bridge, setBridge] = useState<string>("");
  const [tabs, setTabs] = useState<OpenTab[]>([LIBRARY_TAB]);
  const [activeTabId, setActiveTabId] = useState<string>(LIBRARY_TAB_ID);
  const [switching, setSwitching] = useState(false);

  const tabsRef = useRef(tabs);
  const activeTabRef = useRef<OpenTab>(LIBRARY_TAB);
  const switchingRef = useRef(false);
  const switchSeq = useRef(0);

  useEffect(() => {
    tabsRef.current = tabs;
  }, [tabs]);

  useEffect(() => {
    const current = tabs.find((tab) => tab.id === activeTabId) ?? LIBRARY_TAB;
    activeTabRef.current = current;
  }, [tabs, activeTabId]);

  useEffect(() => {
    void api
      .pingVersion()
      .then((version) => setBridge(version || "local"))
      .catch(() => setBridge(""));
  }, []);

  const rememberPosition = (tabID: string, position: number) => {
    setTabs((prev) =>
      prev.map((tab) =>
        tab.id === tabID && tab.kind === "replay" ? { ...tab, position } : tab,
      ),
    );
  };

  /** 切换前取一次当前帧位置；失败时保留上次记录，不阻断切换。 */
  const capturePosition = async (tab: OpenTab): Promise<number> => {
    if (tab.kind !== "replay") return 0;
    try {
      const frame = await api.getFrame();
      return Math.max(0, frame?.now ?? tab.position ?? 0);
    } catch {
      return tab.position ?? 0;
    }
  };

  /** 从卡片 / 侧栏打开事件：总是新开或复用对应回放标签。 */
  const openEvent = async (kind: string, id: string, title: string) => {
    if (switchingRef.current) return;
    switchingRef.current = true;
    setSwitching(true);
    const seq = ++switchSeq.current;
    try {
      const current = activeTabRef.current;
      if (current.kind === "replay") {
        const position = await capturePosition(current);
        rememberPosition(current.id, position);
      }

      const meta = await api.openEvent(kind, id);
      const tabID = `${meta.game_id}:${meta.kind}:${meta.id}`;
      const existing = tabsRef.current.find((tab) => tab.id === tabID);
      const position = existing?.position ?? 0;

      setTabs((prev) => {
        const found = prev.find((tab) => tab.id === tabID);
        if (found) {
          return prev.map((tab) =>
            tab.id === tabID ? { ...tab, meta, title: meta.title || title } : tab,
          );
        }
        return [
          ...prev,
          {
            id: tabID,
            kind: "replay",
            title: meta.title || title,
            kindLabel: meta.kind,
            gameTitle: meta.game_id,
            closable: true,
            meta,
            position: 0,
          },
        ];
      });

      if (position > 0) {
        try {
          await api.seekTo(position);
        } catch {
          /* 重开会话已成功；进度恢复失败按起点播放。 */
        }
      }
      if (seq === switchSeq.current) setActiveTabId(tabID);
    } catch (e: any) {
      alert(e?.message ?? String(e));
    } finally {
      if (seq === switchSeq.current) {
        switchingRef.current = false;
        setSwitching(false);
      }
    }
  };

  /** 切换编辑器标签：后台只有一个回放会话，切换时重开会话并恢复进度。 */
  const selectTab = async (id: string) => {
    if (id === activeTabRef.current.id || switchingRef.current) return;
    const target = tabsRef.current.find((tab) => tab.id === id);
    if (!target) return;

    switchingRef.current = true;
    setSwitching(true);
    const seq = ++switchSeq.current;
    try {
      const current = activeTabRef.current;
      if (current.kind === "replay") {
        const position = await capturePosition(current);
        rememberPosition(current.id, position);
      }

      if (target.kind === "library") {
        setActiveTabId(LIBRARY_TAB_ID);
        await api.exitReplay().catch(() => void 0);
        return;
      }

      if (!target.meta) return;
      const meta = await api.openEvent(target.meta.kind, target.meta.id);
      const position = target.position ?? 0;
      setTabs((prev) =>
        prev.map((tab) => (tab.id === target.id ? { ...tab, meta } : tab)),
      );
      if (position > 0) {
        try {
          await api.seekTo(position);
        } catch {
          /* 会话已开，忽略进度恢复失败。 */
        }
      }
      if (seq === switchSeq.current) setActiveTabId(target.id);
    } catch (e: any) {
      alert(e?.message ?? String(e));
    } finally {
      if (seq === switchSeq.current) {
        switchingRef.current = false;
        setSwitching(false);
      }
    }
  };

  /** 关闭回放标签；关闭当前标签时回到仓库，并显式结束会话。 */
  const closeTab = async (id: string) => {
    const target = tabsRef.current.find((tab) => tab.id === id);
    if (!target?.closable) return;
    const isActive = activeTabRef.current.id === id;
    if (isActive) {
      await capturePosition(target);
      setActiveTabId(LIBRARY_TAB_ID);
      setTabs((prev) => prev.filter((tab) => tab.id !== id));
      await api.exitReplay().catch(() => void 0);
    } else {
      setTabs((prev) => prev.filter((tab) => tab.id !== id));
    }
  };

  const activeTab =
    tabs.find((tab) => tab.id === activeTabId && tab.kind === "replay") ?? null;
  const replay =
    activeTab?.meta != null ? (
      <div className="replay-host" key={activeTab.id}>
        <Replay meta={activeTab.meta} onExit={() => void closeTab(activeTab.id)} />
      </div>
    ) : null;

  return (
    <EventGrid
      bridge={bridge}
      tabs={tabs}
      activeTabId={activeTabId}
      switching={switching}
      replay={replay}
      onOpen={(kind, id, title) => void openEvent(kind, id, title)}
      onSelectTab={(id) => void selectTab(id)}
      onCloseTab={(id) => void closeTab(id)}
    />
  );
}
