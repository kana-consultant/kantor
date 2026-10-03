import { authGetJSON, authPostJSON, authRequestJSON } from "@/lib/api-client";
import { adminRbacKeys } from "@/services/admin-rbac";
import type {
  DocumentMailSetting,
  DocumentMailTestResult,
  UpdateDocumentMailPayload,
} from "@/types/admin";

export const documentMailKeys = {
  setting: () => [...adminRbacKeys.settings(), "document-mail"] as const,
};

export function getDocumentMailSetting() {
  return authGetJSON<DocumentMailSetting>("/admin/settings/document-mail");
}

export function updateDocumentMailSetting(payload: UpdateDocumentMailPayload) {
  return authRequestJSON<DocumentMailSetting>("/admin/settings/document-mail", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function sendDocumentMailTest() {
  return authPostJSON<DocumentMailTestResult, Record<string, never>>(
    "/admin/settings/document-mail/test",
    {},
  );
}
