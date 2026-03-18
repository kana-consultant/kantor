import {
  Facebook,
  Globe,
  Instagram,
  Mail,
  MessageCircle,
  Search,
  Users,
  Youtube,
} from "lucide-react";

import type {
  AdsMetricPlatform,
  Campaign,
  CampaignChannel,
  CampaignStatus,
  LeadPipelineStatus,
  LeadSourceChannel,
} from "@/types/marketing";

export const campaignStatusOptions: Array<{ value: CampaignStatus; label: string }> = [
  { value: "ideation", label: "Ideation" },
  { value: "planning", label: "Planning" },
  { value: "in_production", label: "In Production" },
  { value: "live", label: "Live" },
  { value: "completed", label: "Completed" },
  { value: "archived", label: "Archived" },
];

export const campaignChannelOptions: Array<{ value: CampaignChannel; label: string }> = [
  { value: "instagram", label: "Instagram" },
  { value: "facebook", label: "Facebook" },
  { value: "google_ads", label: "Google Ads" },
  { value: "tiktok", label: "TikTok" },
  { value: "youtube", label: "YouTube" },
  { value: "email", label: "Email" },
  { value: "other", label: "Other" },
];

export const adsMetricPlatformOptions: Array<{ value: AdsMetricPlatform; label: string }> = [
  { value: "instagram", label: "Instagram" },
  { value: "facebook", label: "Facebook" },
  { value: "google_ads", label: "Google Ads" },
  { value: "tiktok", label: "TikTok" },
  { value: "youtube", label: "YouTube" },
  { value: "other", label: "Other" },
];

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

export function channelMeta(channel: CampaignChannel) {
  switch (channel) {
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
    case "google_ads":
      return {
        label: "Google Ads",
        icon: Search,
        badgeClassName: "border-transparent bg-platform-google text-white",
      };
    case "tiktok":
      return {
        label: "TikTok",
        icon: Globe,
        badgeClassName: "border-transparent bg-platform-tiktok text-white",
      };
    case "youtube":
      return {
        label: "YouTube",
        icon: Youtube,
        badgeClassName: "border-transparent bg-platform-youtube text-white",
      };
    case "email":
      return {
        label: "Email",
        icon: Mail,
        badgeClassName: "border-transparent bg-platform-email text-white",
      };
    default:
      return {
        label: "Other",
        icon: Globe,
        badgeClassName: "border-transparent bg-platform-other text-white",
      };
  }
}

export function adsPlatformMeta(platform: AdsMetricPlatform) {
  switch (platform) {
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
    case "google_ads":
      return {
        label: "Google Ads",
        icon: Search,
        badgeClassName: "border-transparent bg-platform-google text-white",
      };
    case "tiktok":
      return {
        label: "TikTok",
        icon: Globe,
        badgeClassName: "border-transparent bg-platform-tiktok text-white",
      };
    case "youtube":
      return {
        label: "YouTube",
        icon: Youtube,
        badgeClassName: "border-transparent bg-platform-youtube text-white",
      };
    default:
      return {
        label: "Other",
        icon: Globe,
        badgeClassName: "border-transparent bg-platform-other text-white",
      };
  }
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
  const apiBase = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";
  const origin = apiBase.replace(/\/api\/v1\/?$/, "");
  return `${origin}/uploads/${filePath}`;
}
