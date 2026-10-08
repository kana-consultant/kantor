import {
  Facebook,
  Globe,
  Instagram,
  Mail,
  Megaphone,
  MessageCircle,
  Search,
  Users,
  Youtube,
  type LucideIcon,
} from "lucide-react";

import { ApiError } from "@/lib/api-client";
import { env } from "@/lib/env";
import type {
  AdsMetricPlatform,
  Campaign,
  CampaignChannel,
  CampaignColumn,
  CampaignStatus,
  LeadPipelineStatus,
  LeadSourceChannel,
} from "@/types/marketing";

// Single source of truth for the campaign channels, ads platforms and campaign
// stages. Types (types/marketing.ts), zod schemas and option lists derive from
// these arrays; the backend keeps the same lists in dto/marketing/options.go.
export const campaignChannels = [
  "meta_ads",
  "instagram",
  "facebook",
  "google_ads",
  "tiktok",
  "youtube",
  "email",
  "other",
] as const;

export const adsMetricPlatforms = [
  "meta_ads",
  "instagram",
  "facebook",
  "google_ads",
  "tiktok",
  "youtube",
  "other",
] as const;

export const campaignStatuses = [
  "ideation",
  "planning",
  "in_production",
  "live",
  "completed",
  "archived",
] as const;

// Default stage names; the board shows the tenant's own column names, these
// are only the fallback when a stage has no column (yet).
const campaignStatusLabels: Record<CampaignStatus, string> = {
  ideation: "Ideation",
  planning: "Planning",
  in_production: "In Production",
  live: "Live",
  completed: "Completed",
  archived: "Archived",
};

export const campaignStatusOptions: Array<{ value: CampaignStatus; label: string }> = campaignStatuses.map((status) => ({
  value: status,
  label: campaignStatusLabels[status],
}));

export function campaignStatusLabel(status: string, columns?: CampaignColumn[] | null) {
  const column = columns?.find((item) => item.stage === status);
  if (column) {
    return column.name;
  }
  return campaignStatusLabels[status as CampaignStatus] ?? humanizeValue(status);
}

// Stage options labelled with the name of the lane that carries each stage, so
// the form says the same thing as the board after a lane was renamed.
export function campaignStageOptions(columns?: CampaignColumn[] | null) {
  return campaignStatuses.map((status) => ({ value: status, label: campaignStatusLabel(status, columns) }));
}

export function isCampaignChannel(value: string): value is CampaignChannel {
  return (campaignChannels as readonly string[]).includes(value);
}

export function isAdsMetricPlatform(value: string): value is AdsMetricPlatform {
  return (adsMetricPlatforms as readonly string[]).includes(value);
}

interface PlatformMeta {
  label: string;
  icon: LucideIcon;
  badgeClassName: string;
  // Small identity dot for ChannelBadge; the only place the platform colour
  // shows in the campaign screens (never as a large fill).
  dotClassName: string;
}

const platformMetaMap: Record<CampaignChannel, PlatformMeta> = {
  meta_ads: { label: "Meta Ads", icon: Megaphone, badgeClassName: "border-transparent bg-platform-meta text-white", dotClassName: "bg-platform-meta" },
  instagram: { label: "Instagram", icon: Instagram, badgeClassName: "border-transparent bg-platform-instagram text-white", dotClassName: "bg-platform-instagram" },
  facebook: { label: "Facebook", icon: Facebook, badgeClassName: "border-transparent bg-platform-facebook text-white", dotClassName: "bg-platform-facebook" },
  google_ads: { label: "Google Ads", icon: Search, badgeClassName: "border-transparent bg-platform-google text-white", dotClassName: "bg-platform-google" },
  // The TikTok colour is black; the ring keeps the dot visible on dark surfaces.
  tiktok: { label: "TikTok", icon: Globe, badgeClassName: "border-transparent bg-platform-tiktok text-white", dotClassName: "bg-platform-tiktok ring-1 ring-text-tertiary" },
  youtube: { label: "YouTube", icon: Youtube, badgeClassName: "border-transparent bg-platform-youtube text-white", dotClassName: "bg-platform-youtube" },
  email: { label: "Email", icon: Mail, badgeClassName: "border-transparent bg-platform-email text-white", dotClassName: "bg-platform-email" },
  other: { label: "Lainnya", icon: Globe, badgeClassName: "border-transparent bg-platform-other text-white", dotClassName: "bg-platform-other" },
};

export const campaignChannelOptions: Array<{ value: CampaignChannel; label: string }> = campaignChannels.map((channel) => ({
  value: channel,
  label: platformMetaMap[channel].label,
}));

export const adsMetricPlatformOptions: Array<{ value: AdsMetricPlatform; label: string }> = adsMetricPlatforms.map((platform) => ({
  value: platform,
  label: platformMetaMap[platform].label,
}));

// A value the frontend does not know (newer backend) shows as itself, never as
// "Lainnya", so it cannot be mistaken for the "other" channel.
export function channelMeta(channel: string): PlatformMeta {
  if (isCampaignChannel(channel)) {
    return platformMetaMap[channel];
  }
  return { ...platformMetaMap.other, label: humanizeValue(channel) };
}

export function adsPlatformMeta(platform: string): PlatformMeta {
  return channelMeta(platform);
}

// Whether an ads platform is a plausible place for a campaign of this channel
// to run. A different platform can be legitimate (a multi-platform campaign, a
// Meta Ads campaign logged per placement), so this only drives an
// informational hint and never blocks a save.
export function platformFitsChannel(channel: string, platform: string) {
  if (!channel || !platform) {
    return true;
  }
  if (channel === "email" || channel === "other" || channel === platform) {
    return true;
  }
  // Instagram and Facebook placements are bought through Meta Ads, YouTube
  // ads through Google Ads, so either side of a family fits the other.
  const families = [
    ["meta_ads", "instagram", "facebook"],
    ["google_ads", "youtube"],
  ];
  return families.some((family) => family.includes(channel) && family.includes(platform));
}

// Short note for tables where a metric's platform does not fit its campaign's
// channel; platformMismatchMessage is the full sentence (form hint, tooltip).
export const platformMismatchNote = "Beda dari kanal campaign";

export function platformMismatchMessage(channel: string, platform: string) {
  if (platformFitsChannel(channel, platform)) {
    return null;
  }
  const channelLabel = channelMeta(channel).label;
  const platformLabel = adsPlatformMeta(platform).label;
  // "Lainnya" is not a place an ad can run, so say where it runs instead.
  const where = platform === "other" ? `di luar ${channelLabel}` : `di ${platformLabel}`;
  return `Kanal campaign ini ${channelLabel}, tapi platform yang dipilih ${platformLabel}. Tetap simpan jika iklannya memang tayang ${where}.`;
}

// The platform a new metric starts with for a campaign: its channel when that
// channel is an ads platform, otherwise "other".
export function defaultPlatformForChannel(channel?: string | null): AdsMetricPlatform {
  return channel && isAdsMetricPlatform(channel) ? channel : "other";
}

export function formatMetricRatio(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return `${value.toFixed(2)}x`;
}

export function formatMetricPercent(value?: number | null) {
  if (value === undefined || value === null) {
    return "-";
  }
  return `${value.toFixed(2)}%`;
}

const shortMonths = ["Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"];

// Compact period for cards and tables: "6–12 Okt 2026", "28 Sep – 4 Okt 2026",
// "28 Des 2026 – 3 Jan 2027". Works on the date part only (no timezone shift).
export function formatShortPeriod(start?: string | null, end?: string | null) {
  const parse = (value?: string | null) => {
    const match = value ? /^(\d{4})-(\d{2})-(\d{2})/.exec(value) : null;
    return match ? { year: Number(match[1]), month: Number(match[2]) - 1, day: Number(match[3]) } : null;
  };
  const from = parse(start);
  const to = parse(end);
  const full = (d: { year: number; month: number; day: number }) => `${d.day} ${shortMonths[d.month]} ${d.year}`;
  if (!from && !to) {
    return "-";
  }
  if (!from || !to) {
    return full((from ?? to)!);
  }
  if (from.year !== to.year) {
    return `${full(from)} – ${full(to)}`;
  }
  if (from.month !== to.month) {
    return `${from.day} ${shortMonths[from.month]} – ${full(to)}`;
  }
  if (from.day === to.day) {
    return full(from);
  }
  return `${from.day}–${to.day} ${shortMonths[to.month]} ${to.year}`;
}

function humanizeValue(value: string) {
  const text = value.replace(/_/g, " ").trim();
  return text ? text.charAt(0).toUpperCase() + text.slice(1) : "-";
}

export const leadSourceOptions: Array<{ value: LeadSourceChannel; label: string }> = [
  { value: "whatsapp", label: "WhatsApp" },
  { value: "email", label: "Email" },
  { value: "instagram", label: "Instagram" },
  { value: "facebook", label: "Facebook" },
  { value: "website", label: "Website" },
  { value: "referral", label: "Referral" },
  { value: "other", label: "Other" },
];

export const leadStatusOptions: Array<{ value: LeadPipelineStatus; label: string }> = [
  { value: "new", label: "New" },
  { value: "contacted", label: "Contacted" },
  { value: "qualified", label: "Qualified" },
  { value: "proposal", label: "Proposal" },
  { value: "negotiation", label: "Negotiation" },
  { value: "won", label: "Won" },
  { value: "lost", label: "Lost" },
];

// --- Error text -------------------------------------------------------------

export interface MarketingErrorInfo {
  message: string;
  // Form field -> Indonesian message, from a VALIDATION_ERROR's details.
  fieldErrors: Record<string, string>;
}

const campaignFieldLabels: Record<string, string> = {
  name: "Nama campaign",
  description: "Deskripsi",
  brief_text: "Teks brief",
  channel: "Kanal",
  status: "Tahap",
  budget_amount: "Anggaran",
  budget_currency: "Mata uang",
  pic_employee_id: "PIC",
  start_date: "Tanggal mulai",
  end_date: "Tanggal selesai",
};

function campaignFieldMessage(field: string, rule: string, columns?: CampaignColumn[] | null, stage?: string) {
  if (field === "end_date" && rule === "before_start_date") {
    return "Tanggal selesai tidak boleh sebelum tanggal mulai";
  }
  if (field === "status" && rule === "stage_unavailable") {
    return `Tahap ${stage ? `"${campaignStatusLabel(stage, columns)}" ` : ""}belum punya kolom di board`;
  }
  if (field === "pic_employee_id") {
    return "PIC tidak ditemukan. Pilih PIC lain.";
  }
  if (field === "name" && (rule === "required" || rule === "min")) {
    return "Nama campaign minimal 3 karakter";
  }
  if (field === "name" && rule === "max") {
    return "Nama campaign maksimal 180 karakter";
  }
  if (field === "description" && rule === "max") {
    return "Deskripsi maksimal 5.000 karakter";
  }
  if (field === "brief_text" && rule === "max") {
    return "Teks brief maksimal 20.000 karakter";
  }
  if (field === "budget_amount" && rule === "min") {
    return "Anggaran tidak boleh negatif";
  }
  if (rule === "required") {
    return `${campaignFieldLabels[field] ?? "Isian ini"} wajib diisi`;
  }
  return `${campaignFieldLabels[field] ?? "Isian ini"} tidak valid`;
}

// Turns any error from the marketing API into Indonesian text, falling back to
// the server message for codes this helper does not know.
export function describeMarketingError(error: unknown, columns?: CampaignColumn[] | null): MarketingErrorInfo {
  if (!(error instanceof ApiError)) {
    return {
      message: error instanceof Error && error.message ? error.message : "Terjadi kesalahan. Coba lagi.",
      fieldErrors: {},
    };
  }

  const details = error.details && typeof error.details === "object" ? (error.details as Record<string, unknown>) : {};
  const stage = typeof details.stage === "string" ? details.stage : undefined;

  switch (error.code) {
    case "VALIDATION_ERROR": {
      const fieldErrors: Record<string, string> = {};
      for (const [field, rule] of Object.entries(details)) {
        if (field in campaignFieldLabels) {
          fieldErrors[field] = campaignFieldMessage(field, String(rule), columns);
        }
      }
      return {
        message: Object.keys(fieldErrors).length > 0 ? "Periksa kembali isian yang ditandai." : error.message,
        fieldErrors,
      };
    }
    case "CAMPAIGN_STAGE_UNAVAILABLE":
      return {
        message: `Tahap "${campaignStatusLabel(stage ?? "", columns)}" belum punya kolom di board, jadi campaign belum bisa disimpan. Coba lagi; jika masih gagal, hubungi admin marketing.`,
        fieldErrors: { status: campaignFieldMessage("status", "stage_unavailable", columns, stage) },
      };
    case "CAMPAIGN_NOT_FOUND":
      return { message: "Campaign tidak ditemukan. Mungkin sudah dihapus; muat ulang halaman.", fieldErrors: {} };
    case "CAMPAIGN_COLUMN_NOT_FOUND":
      return { message: "Kolom board tidak ditemukan. Muat ulang halaman lalu coba lagi.", fieldErrors: {} };
    case "CAMPAIGN_ATTACHMENT_NOT_FOUND":
      return { message: "Lampiran tidak ditemukan. Mungkin sudah dihapus.", fieldErrors: {} };
    case "CAMPAIGN_COLUMN_IN_USE":
      return { message: "Kolom ini masih berisi campaign.", fieldErrors: {} };
    case "CAMPAIGN_COLUMN_PROTECTED":
      return { message: "Kolom tahap tidak bisa dihapus.", fieldErrors: {} };
    case "CAMPAIGN_COLUMN_NAME_TAKEN":
      return { message: "Nama kolom sudah dipakai.", fieldErrors: {} };
    default:
      break;
  }

  if (error.status === 403) {
    return { message: "Akun Anda tidak punya izin untuk tindakan ini.", fieldErrors: {} };
  }
  // 413 comes from the API (JSON) or from the web server in front of it (an
  // HTML page); either way retrying the same upload cannot work.
  if (error.status === 413) {
    return {
      message: "File atau data yang dikirim terlalu besar. Perkecil ukurannya lalu coba lagi.",
      fieldErrors: {},
    };
  }
  // A gateway error without an API code: the proxy could not reach the API.
  if (!error.code && (error.status === 502 || error.status === 503 || error.status === 504)) {
    return { message: "Server sedang tidak tersedia. Coba lagi beberapa saat lagi.", fieldErrors: {} };
  }
  // A JSON error from the API (it has a code) with status 5xx carries an
  // English generic text; errors built by the API client (no code: network,
  // non-JSON body) already carry an Indonesian message.
  if (error.status >= 500 && error.code) {
    return { message: "Terjadi kesalahan di server. Coba lagi beberapa saat lagi.", fieldErrors: {} };
  }
  return { message: error.message || "Terjadi kesalahan. Coba lagi.", fieldErrors: {} };
}

export function marketingErrorMessage(error: unknown, columns?: CampaignColumn[] | null) {
  return describeMarketingError(error, columns).message;
}

// Splits plain text into text and http(s) link parts, for rendering links
// without dangerouslySetInnerHTML.
export function splitTextLinks(text: string): Array<{ type: "text" | "link"; value: string }> {
  const parts: Array<{ type: "text" | "link"; value: string }> = [];
  const pattern = /https?:\/\/[^\s<>"]+/g;
  let lastIndex = 0;
  for (const match of text.matchAll(pattern)) {
    let url = match[0];
    // Leave sentence punctuation after a URL outside the link.
    const trailing = /[.,;:!?)\]]+$/.exec(url)?.[0] ?? "";
    url = url.slice(0, url.length - trailing.length);
    const index = match.index ?? 0;
    if (index > lastIndex) {
      parts.push({ type: "text", value: text.slice(lastIndex, index) });
    }
    parts.push({ type: "link", value: url });
    lastIndex = index + url.length;
  }
  if (lastIndex < text.length) {
    parts.push({ type: "text", value: text.slice(lastIndex) });
  }
  return parts;
}

export function leadSourceMeta(source: LeadSourceChannel) {
  switch (source) {
    case "whatsapp":
      return {
        label: "WhatsApp",
        icon: MessageCircle,
        badgeClassName: "border-transparent bg-platform-whatsapp text-white",
      };
    case "email":
      return {
        label: "Email",
        icon: Mail,
        badgeClassName: "border-transparent bg-platform-email text-white",
      };
    case "instagram":
      return {
        label: "Instagram",
        icon: Instagram,
        badgeClassName: "border-transparent bg-platform-instagram text-white",
      };
    case "facebook":
      return {
        label: "Facebook",
        icon: Facebook,
        badgeClassName: "border-transparent bg-platform-facebook text-white",
      };
    case "website":
      return {
        label: "Website",
        icon: Globe,
        badgeClassName: "border-transparent bg-platform-website text-white",
      };
    case "referral":
      return {
        label: "Referral",
        icon: Users,
        badgeClassName: "border-transparent bg-platform-referral text-white",
      };
    default:
      return {
        label: "Other",
        icon: Globe,
        badgeClassName: "border-transparent bg-platform-other text-white",
      };
  }
}

export function formatLeadStatus(status: LeadPipelineStatus) {
  return status.replace(/_/g, " ");
}

export function formatCampaignStatus(status: CampaignStatus) {
  return status.replace(/_/g, " ");
}

export function initials(value?: string | null) {
  if (!value) {
    return "NA";
  }

  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

export function campaignMatchesFilters(
  campaign: Campaign,
  filters: {
    search: string;
    channel: string;
    status?: string;
    pic: string;
    dateFrom: string;
    dateTo: string;
  },
) {
  const search = filters.search.trim().toLowerCase();
  if (search && !campaign.name.toLowerCase().includes(search)) {
    return false;
  }

  if (filters.channel && campaign.channel !== filters.channel) {
    return false;
  }

  if (filters.status && campaign.status !== filters.status) {
    return false;
  }

  if (filters.pic && campaign.pic_employee_id !== filters.pic) {
    return false;
  }

  if (filters.dateFrom && campaign.end_date.slice(0, 10) < filters.dateFrom) {
    return false;
  }

  if (filters.dateTo && campaign.start_date.slice(0, 10) > filters.dateTo) {
    return false;
  }

  return true;
}

export function uploadsURL(filePath: string) {
  const apiBase = env.VITE_API_BASE_URL;
  const origin = apiBase.replace(/\/api\/v1\/?$/, "");
  return `${origin}/uploads/${filePath}`;
}
