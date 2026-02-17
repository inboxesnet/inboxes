"use client";

import { useParams, useRouter } from "next/navigation";
import { ThreadView } from "@/components/thread-view";

export default function ArchiveThreadPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;
  const domainId = params.domainId as string;

  return (
    <ThreadView
      threadId={threadId}
      domainId={domainId}
      onBack={() => router.push(`/d/${domainId}/archive`)}
    />
  );
}
