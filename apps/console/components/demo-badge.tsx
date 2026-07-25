import { FlaskConical } from "lucide-react";
import { Badge } from "@/components/ui/badge";

/**
 * Visual marker shown whenever a page is rendering the demo dataset because
 * the LedgerCore API was unreachable or not configured.
 */
export function DemoBadge({ demo }: { demo: boolean }) {
  if (!demo) return null;
  return (
    <Badge tone="violet" className="gap-1.5">
      <FlaskConical size={11} aria-hidden="true" />
      datos de demostración
    </Badge>
  );
}
