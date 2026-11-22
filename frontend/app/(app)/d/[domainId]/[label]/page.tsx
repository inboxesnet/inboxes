"use client";

import { useParams } from "next/navigation";
import { ThreadListPage } from "@/components/thread-list-page";
import { decodeRouteParam } from "@/lib/utils";
import type { Label } from "@/lib/types";

export default function LabelPage() {
  const params = useParams();
  const label = decodeRouteParam(params.label as string);

  return <ThreadListPage label={label as Label} title={label} />;
}
