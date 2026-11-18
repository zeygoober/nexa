import { useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { FullScreenSpinner } from "@/components/common/Spinner";
import { nexaTemplateUrl } from "@/lib/nexaTemplate";
import { useUI } from "@/stores/ui";

/** A /template/<code> link: the app, with Import a Template open on it. */
export default function TemplateLinkPage() {
  const { code = "" } = useParams();
  const nav = useNavigate();
  useEffect(() => {
    nav("/channels/@me", { replace: true });
    useUI.getState().openModal("createServer", { template: nexaTemplateUrl(code) });
  }, [code, nav]);
  return <FullScreenSpinner />;
}
