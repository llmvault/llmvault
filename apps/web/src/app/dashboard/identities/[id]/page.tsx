"use client";

import { useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge, type Status } from "@/components/status-badge";
import { ProviderBadge } from "@/components/provider-badge";
import { RemainingBar, RemainingBarCompact } from "@/components/remaining-bar";
import { $api } from "@/api/client";
import type { components } from "@/api/schema";

type CredentialResponse = components["schemas"]["credentialResponse"];

function deriveStatus(cred: CredentialResponse): Status {
  if (cred.revoked_at) return "Revoked";
  if (cred.remaining != null && cred.remaining <= 0) return "Expiring";
  return "Active";
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function formatDateTime(dateStr: string): string {
  return new Date(dateStr).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  });
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toString();
}

function formatDuration(ms: number): string {
  if (ms >= 3_600_000) return `${Math.round(ms / 3_600_000)}h`;
  if (ms >= 60_000) return `${Math.round(ms / 60_000)}m`;
  return `${Math.round(ms / 1_000)}s`;
}

function truncateId(id: string): string {
  if (id.length <= 12) return id;
  return `${id.slice(0, 8)}...${id.slice(-4)}`;
}

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border py-3 last:border-b-0 last:pb-0">
      <span className="text-[13px] text-dim">{label}</span>
      {children}
    </div>
  );
}

type CredentialRow = CredentialResponse & { status: Status };

function CredentialMobileCard({ cred }: { cred: CredentialRow }) {
  const remaining = cred.remaining != null
    ? {
        current: formatCount(cred.remaining),
        max: formatCount(cred.refill_amount ?? cred.remaining),
        percent:
          (cred.refill_amount ?? cred.remaining) > 0
            ? Math.round((cred.remaining / (cred.refill_amount ?? cred.remaining)) * 100)
            : 0,
      }
    : null;

  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="text-[13px] font-medium leading-4 text-foreground">
            {cred.label || "Untitled"}
          </span>
          <span className="font-mono text-[11px] leading-3.5 text-dim">
            {truncateId(cred.id ?? "")}
          </span>
        </div>
        <StatusBadge status={cred.status} />
      </div>
      <div className="flex items-center gap-3">
        {cred.provider_id && <ProviderBadge provider={cred.provider_id} />}
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>{cred.created_at ? formatDate(cred.created_at) : ""}</span>
        {remaining ? (
          <RemainingBarCompact {...remaining} />
        ) : (
          <span className="text-dim">Unlimited</span>
        )}
      </div>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <>
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="h-4 w-32 animate-pulse bg-secondary" />
        <div className="h-7 w-48 animate-pulse bg-secondary" />
      </header>
      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        <div className="flex flex-col gap-5 lg:flex-row">
          <div className="flex-1 h-56 animate-pulse border border-border bg-card" />
          <div className="h-56 animate-pulse border border-border bg-card lg:w-85 lg:shrink-0" />
        </div>
        <div className="h-48 animate-pulse border border-border bg-card" />
      </div>
    </>
  );
}

export default function IdentityDetailPage() {
  const { id } = useParams<{ id: string }>();

  const { data: identity, isLoading: identityLoading } = $api.useQuery(
    "get",
    "/v1/identities/{id}",
    { params: { path: { id } } },
  );

  const { data: credsPage } = $api.useQuery("get", "/v1/credentials", {
    params: {
      query: { identity_id: id, limit: 50 },
    },
  });

  if (identityLoading) return <LoadingSkeleton />;

  if (!identity) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-16">
        <span className="text-sm text-muted-foreground">Identity not found.</span>
        <Link href="/dashboard/identities" className="text-[13px] text-chart-2">
          Back to identities
        </Link>
      </div>
    );
  }

  const ratelimits = identity.ratelimits ?? [];
  const meta = (identity.meta ?? {}) as Record<string, unknown>;
  const credentials: CredentialRow[] = (credsPage?.data ?? []).map((c) => ({
    ...c,
    status: deriveStatus(c),
  }));

  const credentialColumns: DataTableColumn<CredentialRow>[] = [
    {
      id: "label",
      header: "Label",
      width: "22%",
      cell: (row) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium leading-4 text-foreground">
            {row.label || "Untitled"}
          </span>
          <span className="font-mono text-[11px] leading-3.5 text-dim">
            {truncateId(row.id ?? "")}
          </span>
        </div>
      ),
    },
    {
      id: "provider",
      header: "Provider",
      width: "12%",
      cell: (row) =>
        row.provider_id ? (
          <ProviderBadge provider={row.provider_id} />
        ) : (
          <span className="text-xs text-dim">—</span>
        ),
    },
    {
      id: "status",
      header: "Status",
      width: "10%",
      cell: (row) => <StatusBadge status={row.status} />,
    },
    {
      id: "remaining",
      header: "Remaining",
      width: "40%",
      cell: (row) => {
        if (row.status === "Revoked")
          return <span className="text-xs text-dim">—</span>;
        if (row.remaining == null)
          return <span className="text-xs text-dim">Unlimited</span>;
        const max = row.refill_amount ?? row.remaining;
        const percent = max > 0 ? Math.round((row.remaining / max) * 100) : 0;
        return (
          <RemainingBar
            current={formatCount(row.remaining)}
            max={formatCount(max)}
            percent={percent}
          />
        );
      },
    },
    {
      id: "created",
      header: "Created",
      width: "16%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => (row.created_at ? formatDate(row.created_at) : ""),
    },
  ];

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex items-center gap-1.5">
          <Link
            href="/dashboard/identities"
            className="text-[13px] text-dim hover:text-foreground"
          >
            Identities
          </Link>
          <span className="text-[13px] text-dim">/</span>
          <span className="text-[13px] text-muted-foreground">
            {identity.external_id}
          </span>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="font-mono text-lg font-semibold tracking-tight text-foreground sm:text-[22px]">
            {identity.external_id}
          </h1>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="lg">
              Edit
            </Button>
            <Button variant="destructive" size="lg">
              Delete
            </Button>
          </div>
        </div>
      </header>

      {/* Content */}
      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        {/* Info Cards */}
        <div className="flex flex-col gap-5 lg:flex-row">
          {/* Configuration Card */}
          <div className="flex flex-1 flex-col gap-4 border border-border bg-card p-4 sm:p-5">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">
              Configuration
            </span>
            <div className="flex flex-col">
              <ConfigRow label="External ID">
                <span className="font-mono text-[13px] text-foreground">
                  {identity.external_id}
                </span>
              </ConfigRow>
              <ConfigRow label="Credentials">
                <span className="font-mono text-[13px] text-foreground">
                  {credentials.length}
                </span>
              </ConfigRow>
              <ConfigRow label="Requests">
                <span className="font-mono text-[13px] text-foreground">
                  {formatCount(identity.request_count ?? 0)}
                </span>
              </ConfigRow>
              <ConfigRow label="Created">
                <span className="font-mono text-[13px] text-muted-foreground">
                  {identity.created_at
                    ? formatDateTime(identity.created_at)
                    : ""}
                </span>
              </ConfigRow>
              <ConfigRow label="ID">
                <span className="font-mono text-[13px] text-muted-foreground">
                  {identity.id}
                </span>
              </ConfigRow>
            </div>
          </div>

          {/* Rate Limits Card */}
          <div className="flex w-full flex-col gap-4 border border-border bg-card p-4 sm:p-5 lg:w-85 lg:shrink-0">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">
              Rate Limits
            </span>
            {ratelimits.length === 0 ? (
              <span className="py-4 text-center text-[13px] text-dim">
                No rate limits configured
              </span>
            ) : (
              <div className="flex flex-col gap-0">
                {ratelimits.map((rl, i) => (
                  <div
                    key={rl.name ?? i}
                    className={`flex flex-col gap-1.5 py-3 ${i < ratelimits.length - 1 ? "border-b border-border" : ""}`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-[13px] font-medium text-foreground">
                        {rl.name ?? "rate"}
                      </span>
                      <span className="font-mono text-[13px] text-chart-2">
                        {rl.limit != null
                          ? formatCount(rl.limit)
                          : "—"}{" "}
                        / {rl.duration != null ? formatDuration(rl.duration) : "—"}
                      </span>
                    </div>
                    <span className="text-[11px] text-dim">
                      {rl.limit != null && rl.duration != null
                        ? `${formatCount(rl.limit)} ${rl.name ?? "requests"} per ${formatDuration(rl.duration)} window`
                        : ""}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Linked Credentials */}
        <div className="flex flex-col">
          <div className="flex items-center justify-between pb-4">
            <span className="text-sm font-medium text-foreground">
              Linked Credentials
            </span>
            <Link
              href="/dashboard/credentials"
              className="text-[13px] text-chart-2"
            >
              View all credentials
            </Link>
          </div>
          {credentials.length === 0 ? (
            <div className="flex flex-col items-center justify-center border border-border py-12">
              <span className="text-sm text-muted-foreground">
                No credentials linked to this identity
              </span>
            </div>
          ) : (
            <DataTable
              columns={credentialColumns}
              data={credentials}
              keyExtractor={(row) => row.id ?? ""}
              minWidth={700}
              mobileCard={(row) => <CredentialMobileCard cred={row} />}
            />
          )}
        </div>

        {/* Metadata */}
        <div className="flex flex-col">
          <div className="pb-4">
            <span className="text-sm font-medium text-foreground">
              Metadata
            </span>
          </div>
          <div className="border border-border bg-code p-4">
            <pre className="font-mono text-xs leading-5 text-muted-foreground">
              {Object.keys(meta).length > 0
                ? JSON.stringify(meta, null, 2)
                : "{}"}
            </pre>
          </div>
        </div>
      </div>
    </>
  );
}
