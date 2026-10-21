// 根组件：卡墙 ↔ 回放页 两态路由（Bridge 自检静默校验，控制台可见）。
// 路由状态放在这里而非 EventGrid 内：回放页退出时要能回到卡墙原位。

import { useEffect, useState } from "react";
import EventGrid from "@/EventGrid";
import Replay from "@/Replay";
import { api, type SessionMeta } from "@/lib/api";

type View =
  | { page: "grid" }
  | { page: "replay"; meta: SessionMeta };

export default function App() {
  const [bridge, setBridge] = useState<string>("");
  const [view, setView] = useState<View>({ page: "grid" });

  useEffect(() => {
    void api.pingVersion().then(setBridge).catch(() => setBridge(""));
  }, []);
  if (bridge) console.info("[bridge]", bridge);

  const openEvent = async (kind: string, id: string, title: string) => {
    try {
      const meta = await api.openEvent(kind, id);
      void title; // 后端返回的 title 以扫描库为准
      setView({ page: "replay", meta });
    } catch (e: any) {
      alert(e?.message ?? String(e)); // 不静默：构建失败原样可见
    }
  };

  // 返回卡墙：同时关会话（停 BGM/SE）。OpenEvent 换事件时会自开新会话。
  const exitReplay = () => {
    void api.exitReplay().catch(() => void 0);
    setView({ page: "grid" });
  };

  return view.page === "grid" ? (
    <EventGrid onOpen={openEvent} />
  ) : (
    <Replay meta={view.meta} onExit={exitReplay} />
  );
}
