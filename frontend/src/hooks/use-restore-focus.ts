import { useEffect } from "react";

// Gives focus back to the element that had it when a modal (dialog, drawer)
// opened, once the modal closes, so a keyboard user keeps their place. Only
// when focus was dropped to <body> or is still inside the closing modal
// (data-state="closed" on the panel, data-modal-root="closed" on its portal
// root, during its exit animation): a modal that opened in the
// meantime keeps its focus.
export function useRestoreFocus(open: boolean) {
  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const openerId = opener?.id;

    return () => {
      if (!opener || opener === document.body) {
        return;
      }
      // The opener may have been re-rendered while the modal was open (a
      // board card that moved lanes); an element with the same id stands in.
      const target = opener.isConnected ? opener : openerId ? document.getElementById(openerId) : null;
      if (!target) {
        return;
      }
      // Runs after the commit: the modal is already removed (focus fell to
      // <body>) or marked data-state="closed" for its exit animation.
      const active = document.activeElement;
      // preventScroll: giving focus back must never move the page.
      // A click on the backdrop focuses the backdrop button, a sibling of the
      // [role=dialog] panel, so the closing modal's portal root
      // (data-modal-root="closed") counts as "inside the modal" too.
      if (
        !active ||
        active === document.body ||
        active.closest('[role="dialog"][data-state="closed"], [data-modal-root="closed"]')
      ) {
        target.focus({ preventScroll: true });
      }
    };
  }, [open]);
}
