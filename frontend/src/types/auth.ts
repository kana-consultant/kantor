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
  refresh_token: string;
  token_type: string;
  expires_in: number;
}

export interface AuthSession {
  user: AuthUser;
  roles: string[];
  permissions: string[];
  tokens: AuthTokens;
}

export interface AuthPayload {
  user: AuthUser;
  roles: string[];
  permissions: string[];
  tokens: AuthTokens;
}
