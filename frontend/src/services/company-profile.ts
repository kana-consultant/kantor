import { authDownload, authRequestJSON } from "@/lib/api-client";
import { adminRbacKeys } from "@/services/admin-rbac";
import type { CompanyProfile, UpdateCompanyProfilePayload } from "@/types/admin";

/** Upload limit of the tenant logo, mirrored from the backend. */
export const COMPANY_LOGO_MAX_BYTES = 2 * 1024 * 1024;
export const COMPANY_LOGO_TYPES = ["image/png", "image/jpeg"] as const;

export const companyProfileKeys = {
  profile: () => [...adminRbacKeys.settings(), "company-profile"] as const,
  logo: (updatedAt: string | null) =>
    [...companyProfileKeys.profile(), "logo", updatedAt ?? "none"] as const,
};

export function getCompanyProfile() {
  return authRequestJSON<CompanyProfile>("/admin/settings/company-profile", { method: "GET" });
}

export function updateCompanyProfile(payload: UpdateCompanyProfilePayload) {
  return authRequestJSON<CompanyProfile>("/admin/settings/company-profile", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

/** Uploads a PNG/JPEG logo. The server trims, downscales and stores it as PNG. */
export function uploadCompanyLogo(file: File) {
  const formData = new FormData();
  formData.set("file", file);
  return authRequestJSON<CompanyProfile>("/admin/settings/company-profile/logo", {
    method: "POST",
    body: formData,
  });
}

export function deleteCompanyLogo() {
  return authRequestJSON<CompanyProfile>("/admin/settings/company-profile/logo", {
    method: "DELETE",
  });
}

/** Fetches the processed logo (PNG). The caller owns and must revoke any object URL. */
export async function fetchCompanyLogo() {
  const { blob } = await authDownload("/admin/settings/company-profile/logo", { method: "GET" });
  return blob;
}
