import { onBeforeUnmount, ref } from "vue";

export function useStatusMessage(successTimeoutMs = 5000) {
  const successMessage = ref("");
  const errorMessage = ref("");
  let successTimer: number | null = null;

  function clearSuccess(): void {
    if (successTimer !== null) {
      window.clearTimeout(successTimer);
      successTimer = null;
    }
    successMessage.value = "";
  }

  function setSuccess(message: string): void {
    clearSuccess();
    successMessage.value = message;

    if (!message.trim() || typeof window === "undefined") {
      return;
    }

    successTimer = window.setTimeout(() => {
      successMessage.value = "";
      successTimer = null;
    }, successTimeoutMs);
  }

  function clearError(): void {
    errorMessage.value = "";
  }

  function setError(message: string): void {
    errorMessage.value = message;
  }

  function clearAll(): void {
    clearSuccess();
    clearError();
  }

  onBeforeUnmount(() => {
    if (successTimer !== null) {
      window.clearTimeout(successTimer);
      successTimer = null;
    }
  });

  return {
    successMessage,
    errorMessage,
    setSuccess,
    setError,
    clearSuccess,
    clearError,
    clearAll,
  };
}
