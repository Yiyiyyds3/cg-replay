import { useEffect, useState } from "react";
import { Ship, CheckCircle2, XCircle } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

// M1 骨架自检页：验证 Go 绑定桥与样式体系都通。
// M2 在此位置换成事件网格（EventGrid），路由不再改。
export default function App() {
  const [version, setVersion] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .pingVersion()
      .then(setVersion)
      .catch((e) => setError(e?.message ?? String(e)));
  }, []);

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-4 rounded-lg border border-border bg-card px-12 py-10 text-center shadow-lg">
        <Ship className="h-10 w-10 text-primary" />
        <h1 className="text-xl font-semibold">重返蓝鲸岛 · CG 演出回放</h1>
        <p className="text-sm text-muted-foreground">
          方案 B（Wails v2 + React）骨架已就绪
        </p>
        {version !== null && (
          <div className="flex items-center gap-2 text-sm">
            <CheckCircle2 className="h-4 w-4 text-green-500" />
            <span className="font-mono text-muted-foreground">{version}</span>
          </div>
        )}
        {error !== null && (
          <Badge variant="warn" className="px-2 py-1 text-xs">
            <XCircle className="mr-1 h-3 w-3" />
            绑定桥异常：{error}
          </Badge>
        )}
        <Button variant="outline" size="sm" className="mt-2">
          M2 预留：进入事件网格
        </Button>
      </div>
    </div>
  );
}
