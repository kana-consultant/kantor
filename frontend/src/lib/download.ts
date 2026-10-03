/**
 * Saves a Blob as a file through a temporary object URL. The URL is revoked
 * on the next tick: revoking synchronously after click() can cancel the
 * download in some browsers.
 */
export function triggerDownload(blob: Blob, filename: string) {
  const objectUrl = window.URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = filename;
  anchor.rel = "noopener";
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => window.URL.revokeObjectURL(objectUrl), 0);
}
