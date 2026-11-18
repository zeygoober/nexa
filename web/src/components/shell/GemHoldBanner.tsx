import { Button } from "@/components/common/Button";
import { GemIcon } from "@/components/gems/GemIcon";
import { t } from "@/i18n";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import s from "./MaintenanceBanner.module.css";

/** While a farming check holds new Gems, a line under the other banners until it is answered on the Gems page. */
export function GemHoldBanner() {
  const state = useAuth((a) => a.me?.gem_state);
  const openModal = useUI((u) => u.openModal);
  if (state !== "check") return null;
  return (
    <div className={s.banner} data-testid="gems-hold-banner" role="region" aria-label={t("gems.title")}>
      <GemIcon size={20} />
      <div className={s.words}><span>{t("gems.banner")}</span></div>
      <Button size="sm" variant="secondary" className={s.action} onClick={() => openModal("settings", { page: "gems" })} data-testid="gem-hold-open">{t("gems.hold.confirm")}</Button>
    </div>
  );
}
