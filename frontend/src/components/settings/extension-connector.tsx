import { useEffect, useState } from "react";
import { PlugZap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { connectExtension, disconnectExtension, pingExtension } from "@/services/extension";
import { env } from "@/lib/env";
import { toast } from "@/stores/toast-store";
import type { ExtensionStatus } from "@/services/extension";

export function ExtensionConnector() {
  const [status, setStatus] = useState<ExtensionStatus | "loading">("loading");
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    pingExtension().then((result) => setStatus(result));
  }, []);

  async function handleConnect() {
    setIsLoading(true);
    try {
      const apiBaseUrl = env.VITE_API_BASE_URL;
      await connectExtension(apiBaseUrl, window.location.origin);
      setStatus("connected");
      toast.success("Extension terhubung", "Chrome extension sudah terhubung ke akun Anda.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal menghubungkan extension");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleDisconnect() {
    setIsLoading(true);
    try {
      await disconnectExtension();
      setStatus("disconnected");
      toast.success("Extension terputus", "Chrome extension sudah terputus dari akun Anda.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Gagal memutuskan extension");
    } finally {
      setIsLoading(false);
    }
  }

  if (status === "loading") {
    return null;
  }

  return (
    <Card className="p-6">
      <div className="flex items-center gap-3 mb-4">
        <PlugZap className="h-5 w-5 text-muted-foreground" />
        <h3 className="text-lg font-semibold">Chrome Extension</h3>
      </div>

      {status === "not_installed" && (
        <div>
          <p className="text-sm text-muted-foreground mb-3">
            KANTOR Activity Tracker extension belum terdeteksi di browser ini.
          </p>
          <p className="text-sm text-muted-foreground">
            Instal extension dari halaman Operational &rarr; Activity Tracker, lalu kembali ke sini.
          </p>
        </div>
      )}

      {status === "disconnected" && (
        <div>
          <p className="text-sm text-muted-foreground mb-3">
            Extension terinstal tapi belum terhubung ke akun Anda.
          </p>
          <Button onClick={handleConnect} disabled={isLoading}>
            {isLoading ? "Menghubungkan..." : "Hubungkan Extension"}
          </Button>
        </div>
      )}

      {status === "connected" && (
        <div>
          <p className="text-sm text-muted-foreground mb-3">
            Extension terhubung ke akun Anda.
          </p>
          <Button variant="outline" onClick={handleDisconnect} disabled={isLoading}>
            {isLoading ? "Memutuskan..." : "Putuskan Extension"}
          </Button>
        </div>
      )}
    </Card>
  );
}
