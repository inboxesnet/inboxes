"use client";

import { useParams, useRouter } from "next/navigation";
import { ThreadView } from "@/components/thread-view";
import { decodeRouteParam } from "@/lib/utils";

export default function ThreadDetailPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;
  const domainId = params.domainId as string;
  const label = decodeRouteParam(params.label as string);

  return (
    <ThreadView
      threadId={threadId}
      domainId={domainId}
      label={label}
      onBack={() => router.push(`/d/${domainId}/${encodeURIComponent(label)}`)}
    />
  );
}
