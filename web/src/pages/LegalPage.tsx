import { Link } from "react-router-dom";
import { NexaWordmark } from "@/components/common/Brand";
import { Spinner } from "@/components/common/Spinner";
import { ContactBlock, LegalHighlights, OpenSourceBlock } from "@/components/legal/LegalContent";
import { LegalDocument } from "@/components/legal/LegalDocument";
import { CONTACT_HREF } from "@/lib/legal";
import { useLegalDocs } from "@/lib/legalDocs";
import { hasSavedSession, useAuth } from "@/stores/auth";
import { t } from "@/i18n";
import { usePageTitle } from "./AuthLayout";
import s from "./Auth.module.css";
import l from "@/components/legal/Legal.module.css";

/**
 * The public legal pages: /terms and /privacy show the documents whole, as
 * written in docs/legal; /about says what Nexa is and points at both. The
 * documents are English on purpose — a translated Terms is a different Terms.
 */
export default function LegalPage({ page }: { page: "terms" | "privacy" | "about" }) {
  const docs = useLegalDocs(page !== "about");
  // someone signed in (who came from Settings, say) is offered the app, not a sign-up
  const signedIn = useAuth((a) => !!a.me) || hasSavedSession();
  usePageTitle(page === "terms" ? t("legal.termsTitle") : page === "privacy" ? t("legal.privacyTitle") : t("legal.aboutTitle"));
  // /about already links both documents and the contact just above
  const links = [
    ...(signedIn ? [<Link key="open" to="/channels/@me">{t("home.nav.open")}</Link>] : [<Link key="start" to="/register">{t("home.nav.getStarted")}</Link>, <Link key="login" to="/login">{t("home.nav.logIn")}</Link>]),
    ...(page === "privacy" ? [<Link key="terms" to="/terms">{t("legal.termsTitle")}</Link>] : []),
    ...(page === "terms" ? [<Link key="privacy" to="/privacy">{t("legal.privacyTitle")}</Link>] : []),
    ...(page !== "about" ? [<Link key="about" to="/about">{t("legal.aboutTitle")}</Link>, <a key="contact" href={CONTACT_HREF}>{t("legal.contactOwner")}</a>] : []),
  ];
  return (
    <div className={s.page}>
      <div className={s.brand}>
        <Link to="/" aria-label="Nexa">
          <NexaWordmark height={38} />
        </Link>
      </div>
      <main className={s.card} style={{ width: 760 }} data-testid={`legal-${page}`}>
        {page !== "about" && !docs ? (
          <div className={l.loading}>
            <Spinner />
          </div>
        ) : null}
        {page === "terms" && docs ? <LegalDocument doc={docs.terms} page /> : null}
        {page === "privacy" && docs ? <LegalDocument doc={docs.privacy} page /> : null}
        {page === "about" ? (
          <>
            <h1 className={s.title}>{t("legal.aboutTitle")}</h1>
            <p className={l.body} dir="auto">{t("legal.aboutLead")}</p>
            <OpenSourceBlock />
            <LegalHighlights />
            <p className={l.body} style={{ marginTop: 16 }}>
              <Link to="/terms">{t("legal.termsTitle")}</Link>
              {" · "}
              <Link to="/privacy">{t("legal.privacyTitle")}</Link>
            </p>
            <ContactBlock />
          </>
        ) : null}
        <nav className={l.pageLinks}>
          {links}
        </nav>
      </main>
    </div>
  );
}
