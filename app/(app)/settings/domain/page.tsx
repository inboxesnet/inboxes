"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Copy, CheckCircle, Loader2 } from "lucide-react";

interface DnsRecord {
  type: string;
  name: string;
  value: string;
  priority: number | null;
  status: string;
}

interface DomainInfo {
  id: string;
  domain: string;
  status: string;
  mx_verified: boolean;
  spf_verified: boolean;
  dkim_verified: boolean;
  verified_at: string | null;
}

export default function DomainSettingsPage() {
  const [domain, setDomain] = useState<DomainInfo | null>(null);
  const [dnsRecords, setDnsRecords] = useState<DnsRecord[]>([]);
  const [domainInput, setDomainInput] = useState("");
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [verifying, setVerifying] = useState(false);
  const [error, setError] = useState("");
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null);

  useEffect(() => {
    fetchDomain();
  }, []);

  async function fetchDomain() {
    try {
      const res = await fetch("/api/domains");
      if (res.ok) {
        const data = await res.json();
        setDomain(data.domain);
      }
    } catch {
      setError("Failed to load domain information");
    } finally {
      setLoading(false);
    }
  }

  async function handleAddDomain(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setAdding(true);

    try {
      const res = await fetch("/api/domains", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domain: domainInput }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Failed to add domain");
        return;
      }

      setDomain(data.domain);
      setDnsRecords(data.dns_records || []);
      setDomainInput("");
    } catch {
      setError("Failed to add domain");
    } finally {
      setAdding(false);
    }
  }

  async function handleVerify() {
    if (!domain) return;
    setError("");
    setVerifying(true);

    try {
      const res = await fetch(`/api/domains/${domain.id}/verify`, {
        method: "POST",
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Verification failed");
        return;
      }

      setDomain(data.domain);
      setDnsRecords(data.records || []);
    } catch {
      setError("Failed to verify domain");
    } finally {
      setVerifying(false);
    }
  }

  async function handleCopy(value: string, index: number) {
    try {
      await navigator.clipboard.writeText(value);
      setCopiedIndex(index);
      setTimeout(() => setCopiedIndex(null), 2000);
    } catch {
      // Fallback for older browsers
    }
  }

  function getStatusBadge(status: string) {
    switch (status) {
      case "verified":
        return <Badge variant="success">Verified</Badge>;
      case "not_started":
      case "pending":
        return <Badge variant="warning">Pending</Badge>;
      default:
        return <Badge variant="destructive">Failed</Badge>;
    }
  }

  function getDomainStatusBadge(status: string) {
    switch (status) {
      case "active":
        return <Badge variant="success">Active</Badge>;
      case "verified":
        return <Badge variant="success">Verified</Badge>;
      case "pending":
        return <Badge variant="warning">Pending</Badge>;
      default:
        return <Badge variant="destructive">{status}</Badge>;
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Domain Settings</h2>
        <p className="mt-1 text-muted-foreground">
          Configure your custom email domain.
        </p>
      </div>

      {error && (
        <div className="rounded-md bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {!domain ? (
        <Card>
          <CardHeader>
            <CardTitle>Add Your Domain</CardTitle>
            <CardDescription>
              Enter your domain name to get started with custom email addresses.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleAddDomain} className="flex gap-3">
              <div className="flex-1">
                <Label htmlFor="domain" className="sr-only">
                  Domain
                </Label>
                <Input
                  id="domain"
                  type="text"
                  placeholder="example.com"
                  value={domainInput}
                  onChange={(e) => setDomainInput(e.target.value)}
                  disabled={adding}
                />
              </div>
              <Button type="submit" disabled={adding || !domainInput.trim()}>
                {adding && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Add Domain
              </Button>
            </form>
          </CardContent>
        </Card>
      ) : (
        <>
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle>{domain.domain}</CardTitle>
                  <CardDescription className="mt-1">
                    Configure the DNS records below to activate your domain.
                  </CardDescription>
                </div>
                {getDomainStatusBadge(domain.status)}
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex gap-2 text-sm">
                <span className="text-muted-foreground">MX:</span>
                {domain.mx_verified ? (
                  <Badge variant="success">Verified</Badge>
                ) : (
                  <Badge variant="warning">Pending</Badge>
                )}
                <span className="ml-2 text-muted-foreground">SPF:</span>
                {domain.spf_verified ? (
                  <Badge variant="success">Verified</Badge>
                ) : (
                  <Badge variant="warning">Pending</Badge>
                )}
                <span className="ml-2 text-muted-foreground">DKIM:</span>
                {domain.dkim_verified ? (
                  <Badge variant="success">Verified</Badge>
                ) : (
                  <Badge variant="warning">Pending</Badge>
                )}
              </div>

              <Button
                onClick={handleVerify}
                disabled={verifying || domain.status === "active"}
                variant={domain.status === "active" ? "secondary" : "default"}
              >
                {verifying && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {domain.status === "active" ? "Domain Verified" : "Verify Domain"}
              </Button>
            </CardContent>
          </Card>

          {dnsRecords.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>DNS Records</CardTitle>
                <CardDescription>
                  Add these records to your domain&apos;s DNS configuration.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b">
                        <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                          Type
                        </th>
                        <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                          Name
                        </th>
                        <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                          Value
                        </th>
                        <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                          Priority
                        </th>
                        <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                          Status
                        </th>
                        <th className="py-3 text-left font-medium text-muted-foreground">
                          &nbsp;
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {dnsRecords.map((record, index) => (
                        <tr key={index} className="border-b last:border-0">
                          <td className="py-3 pr-4 font-mono">{record.type}</td>
                          <td className="py-3 pr-4 max-w-[200px] truncate font-mono text-xs">
                            {record.name}
                          </td>
                          <td className="py-3 pr-4 max-w-[300px] truncate font-mono text-xs">
                            {record.value}
                          </td>
                          <td className="py-3 pr-4">
                            {record.priority !== null ? record.priority : "-"}
                          </td>
                          <td className="py-3 pr-4">
                            {getStatusBadge(record.status)}
                          </td>
                          <td className="py-3">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleCopy(record.value, index)}
                              title="Copy value"
                            >
                              {copiedIndex === index ? (
                                <CheckCircle className="h-4 w-4 text-green-600" />
                              ) : (
                                <Copy className="h-4 w-4" />
                              )}
                            </Button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  );
}
