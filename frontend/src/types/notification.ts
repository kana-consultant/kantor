export interface NotificationItem {
  id: string;
  user_id: string;
  type: string;
  title: string;
  message: string;
  is_read: boolean;
  reference_type?: string | null;
  reference_id?: string | null;
  created_at: string;
}

export interface NotificationFilters {
  page?: number;
  perPage?: number;
  read?: boolean;
}
