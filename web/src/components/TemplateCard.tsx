import { Link } from "react-router-dom";
import { relativeTime } from "../api";
import type { TemplateData } from "../types";
import Icon from "./Icon";
import { KindBadge, VisibilityBadge } from "./ui";

interface TemplateCardProps {
  template: TemplateData;
  mine?: boolean;
}

// The two letters on a card's tile: the template's own initials, so a grid of
// cards is told apart at a glance without an image upload.
export function initials(name: string): string {
  const words = name.split(/[\s_-]+/).filter((word) => word !== "");
  const letters = words.length > 1 ? `${words[0][0]}${words[1][0]}` : name.slice(0, 2);
  return letters.toUpperCase();
}

export default function TemplateCard({ template, mine }: TemplateCardProps) {
  const files = template.file_count === 1 ? "1 file" : `${template.file_count} files`;
  const updated = relativeTime(template.updated_at);

  return (
    <article className="tcard">
      <div className="tcard-top">
        <span className={`tile kind-${template.kind}`} aria-hidden="true">
          {initials(template.name)}
        </span>
        <div className="tcard-badges">
          {mine === true ? <span className="badge badge-mine">Yours</span> : null}
          <VisibilityBadge isPublic={template.is_public} />
        </div>
      </div>
      <h3 className="tcard-title">
        {/* The whole card is the link's hit area, via the stretched ::after. */}
        <Link to={`/templates/${template.id}`} className="stretched">
          {template.name}
        </Link>
      </h3>
      <p className="tcard-body">
        {template.description === "" ? "No description." : template.description}
      </p>
      <footer className="tcard-foot">
        <KindBadge kind={template.kind} />
        <span className="tcard-meta">
          <Icon name="file" size={14} /> {files}
        </span>
        <span className="tcard-meta">v{template.version}</span>
        {updated !== "" ? <span className="tcard-meta tcard-time">{updated}</span> : null}
      </footer>
    </article>
  );
}
