"use client";

import { useParams, useRouter } from "next/navigation";
import { ThreadView } from "@/components/thread-view";

export default function InboxThreadPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;
  const domainId = params.domainId as string;

  return (
    <ThreadView
      threadId={threadId}
      domainId={domainId}
      label="inbox"
      onBack={() => router.push(`/d/${domainId}/inbox`)}
    />
  );
}
