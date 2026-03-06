"use client";

import { useParams, useRouter } from "next/navigation";
import { ThreadView } from "@/components/thread-view";

export default function SentThreadPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;
  const domainId = params.domainId as string;

  return (
    <ThreadView
      threadId={threadId}
      domainId={domainId}
      label="sent"
      onBack={() => router.push(`/d/${domainId}/sent`)}
    />
  );
}
