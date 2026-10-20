import { useEffect, useState } from "react";
import EventGrid from "@/EventGrid";
import { api } from "@/lib/api";

// 根组件:M1 的自检数据在此静默校验（控制台可见）,渲染交给事件网格 M2。
export default function App() {
  const [bridge, setBridge] = useState<string>("");
  useEffect(() => {
    void api.pingVersion().then(setBridge).catch(() => setBridge(""));
  }, []);
  if (bridge) console.info("[bridge]", bridge);
  return <EventGrid />;
}
