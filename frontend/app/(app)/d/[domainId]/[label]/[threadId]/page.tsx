"use client";

import { useParams, useRouter } from "next/navigation";
import { ThreadView } from "@/components/thread-view";

export default function ThreadDetailPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;
  const domainId = params.domainId as string;
  const label = params.label as string;

  return (
    <ThreadView
      threadId={threadId}
      domainId={domainId}
      label={label}
      onBack={() => router.push(`/d/${domainId}/${label}`)}
    />
  );
}
