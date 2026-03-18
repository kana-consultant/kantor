import { create } from "zustand";

export type ToastTone = "info" | "success" | "warning" | "error";

export interface ToastItem {
  id: string;
  title: string;
  description?: string;
  tone: ToastTone;
}

interface ToastState {
  items: ToastItem[];
  push: (item: Omit<ToastItem, "id">) => string;
  dismiss: (id: string) => void;
  clear: () => void;
}

function createToastID() {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

export const useToastStore = create<ToastState>()((set) => ({
  items: [],
  push: (item) => {
    const id = createToastID();
    set((state) => ({
      items: [...state.items, { id, ...item }],
    }));
    return id;
  },
  dismiss: (id) =>
    set((state) => ({
      items: state.items.filter((item) => item.id !== id),
    })),
  clear: () => set({ items: [] }),
}));

export const toast = {
  info(title: string, description?: string) {
    return useToastStore.getState().push({ tone: "info", title, description });
  },
  success(title: string, description?: string) {
    return useToastStore.getState().push({ tone: "success", title, description });
  },
  warning(title: string, description?: string) {
    return useToastStore.getState().push({ tone: "warning", title, description });
  },
  error(title: string, description?: string) {
    return useToastStore.getState().push({ tone: "error", title, description });
  },
  dismiss(id: string) {
    useToastStore.getState().dismiss(id);
  },
};
