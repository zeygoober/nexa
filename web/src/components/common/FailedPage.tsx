import { useState, type ComponentType } from "react";
import { LoadError } from "@/components/common/LoadError";
import { Modal } from "@/components/common/Modal";
import { t } from "@/i18n";

/* eslint-disable @typescript-eslint/no-explicit-any */

/**
 * Stands in for a page whose code could not be fetched, with a way to ask
 * again that keeps the person where they are. Once a try works it is the real
 * page, here and whenever it opens next. `dialog` is for a whole dialog, which
 * has no frame of its own to show this in yet.
 */
export function failedPage<T extends ComponentType<any>>(retry: () => Promise<{ default: T }>, dialog = false): T {
  let Real: ComponentType<any> | null = null;
  function FailedPage(props: { onClose?: () => void }) {
    const [busy, setBusy] = useState(false);
    if (Real) return <Real {...props} />;
    const again = () => {
      setBusy(true);
      retry().then(
        (m) => {
          Real = m.default;
          setBusy(false);
        },
        () => setBusy(false),
      );
    };
    const message = <LoadError center message={t("server.serverSettings.pageCouldNotLoad")} onRetry={again} busy={busy} />;
    return dialog ? (
      <Modal open onClose={props.onClose ?? (() => undefined)} ariaLabel={t("server.serverSettings.serverSettings")} width={320}>
        {message}
      </Modal>
    ) : (
      message
    );
  }
  return FailedPage as unknown as T;
}
