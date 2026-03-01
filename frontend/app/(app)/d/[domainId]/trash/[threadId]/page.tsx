"use client";

import { useParams, useRouter } from "next/navigation";
import { ThreadView } from "@/components/thread-view";

export default function TrashThreadPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;
  const domainId = params.domainId as string;

  return (
    <ThreadView
      threadId={threadId}
      domainId={domainId}
      label="trash"
      onBack={() => router.push(`/d/${domainId}/trash`)}
    />
  );
}
