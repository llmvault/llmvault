import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { CredConfig } from "./credential-config";

export function CredentialFieldsForm({
  config,
  values,
  onChange,
  idPrefix = "cred",
  placeholderPrefix = "Enter",
}: {
  config: CredConfig;
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  idPrefix?: string;
  placeholderPrefix?: string;
}) {
  if (config.fields.length === 0) {
    return config.message ? (
      <div className="border-t border-border pt-4">
        <span className="text-xs text-muted-foreground">{config.message}</span>
      </div>
    ) : null;
  }

  return (
    <div className="flex flex-col gap-3 border-t border-border pt-4">
      <span className="text-xs font-medium text-muted-foreground">
        Credentials
      </span>
      {config.message && (
        <span className="text-[11px] text-dim">{config.message}</span>
      )}
      {config.fields.map((f) => {
        const isOptional = config.optional?.includes(f.key);
        const inputId = `${idPrefix}-${f.key}`;
        const placeholder = `${placeholderPrefix} ${f.label.toLowerCase()}...`;

        return (
          <div key={f.key} className="flex flex-col gap-1.5">
            <Label htmlFor={inputId} className="text-xs">
              {f.label}{" "}
              {!isOptional && <span className="text-destructive">*</span>}
            </Label>
            {f.type === "textarea" ? (
              <textarea
                id={inputId}
                value={values[f.key] ?? ""}
                onChange={(e) => onChange(f.key, e.target.value)}
                className="flex min-h-20 w-full border border-input bg-background px-3 py-2 font-mono text-[13px] ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                placeholder={placeholder}
              />
            ) : (
              <Input
                id={inputId}
                value={values[f.key] ?? ""}
                onChange={(e) => onChange(f.key, e.target.value)}
                className="h-10 font-mono text-[13px]"
                placeholder={placeholder}
                type={
                  f.key.includes("secret") || f.key === "private_key"
                    ? "password"
                    : "text"
                }
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
