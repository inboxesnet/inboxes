"use client";

import { ThreadListPage } from "@/components/thread-list-page";

export default function TrashPage() {
  return (
    <ThreadListPage
      label="trash"
      title="Trash"
      subtitle="Messages are permanently deleted after 30 days"
    />
  );
}
