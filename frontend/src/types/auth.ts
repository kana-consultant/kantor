export interface AuthUser {
  id: string;
  email: string;
  full_name: string;
  avatar_url?: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface AuthTokens {
  access_token: string;
  token_type: string;
  expires_in: number;
}

export interface AuthModuleRole {
  role_id: string;
  role_name: string;
  role_slug: string;
}

export interface TenantInfo {
  id: string;
  slug: string;
  name: string;
}

export interface AuthSession {
  user: AuthUser;
  tenant?: TenantInfo;
  module_roles: Record<string, AuthModuleRole>;
  permissions: string[];
  is_super_admin: boolean;
  tokens: AuthTokens;
}

export type AuthPayload = AuthSession;
