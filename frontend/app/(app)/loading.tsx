import { Spinner } from "@/components/ui/spinner";

export default function Loading() {
  return (
    <div className="flex items-center justify-center h-dvh">
      <Spinner className="h-8 w-8" />
    </div>
  );
}
