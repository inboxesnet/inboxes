"use client";

import { useParams } from "next/navigation";
import { useDomains } from "@/contexts/domain-context";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export default function DomainSettingsPage() {
  const params = useParams();
  const domainId = params.domainId as string;
  const { domains } = useDomains();
  const domain = domains.find((d) => d.id === domainId);

  if (!domain) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        Domain not found
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-6 space-y-6">
      <h1 className="text-2xl font-semibold">Domain Settings</h1>

      <Card>
        <CardHeader>
          <CardTitle>{domain.domain}</CardTitle>
          <CardDescription>DNS verification status</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm">Status</span>
            <Badge
              variant={
                domain.status === "active"
                  ? "default"
                  : domain.status === "verified"
                    ? "secondary"
                    : "outline"
              }
            >
              {domain.status}
            </Badge>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm">MX Records</span>
            <Badge variant={domain.mx_verified ? "default" : "destructive"}>
              {domain.mx_verified ? "Verified" : "Not verified"}
            </Badge>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm">SPF</span>
            <Badge variant={domain.spf_verified ? "default" : "destructive"}>
              {domain.spf_verified ? "Verified" : "Not verified"}
            </Badge>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm">DKIM</span>
            <Badge variant={domain.dkim_verified ? "default" : "destructive"}>
              {domain.dkim_verified ? "Verified" : "Not verified"}
            </Badge>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
