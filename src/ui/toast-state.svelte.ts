// One toast at a time. Auto-dismiss after 5 s, paused while hovered or focused.
export const toastState = $state({ message: '', id: 0 });

export function showToast(message: string) {
  toastState.message = message;
  toastState.id++;
}
export function clearToast() { toastState.message = ''; }
