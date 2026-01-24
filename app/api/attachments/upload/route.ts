import { NextRequest, NextResponse } from "next/server";
import { S3Client, PutObjectCommand } from "@aws-sdk/client-s3";
import { getCurrentUser } from "@/lib/session";
import { randomUUID } from "crypto";

const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50MB
const MAX_TOTAL_SIZE = 50 * 1024 * 1024; // 50MB total per email

function getS3Client() {
  const accessKeyId = process.env.AWS_ACCESS_KEY_ID;
  const secretAccessKey = process.env.AWS_SECRET_ACCESS_KEY;
  const region = process.env.AWS_REGION || "us-east-1";
  const endpoint = process.env.S3_ENDPOINT; // For S3-compatible services like MinIO

  if (!accessKeyId || !secretAccessKey) {
    throw new Error("S3 credentials not configured");
  }

  return new S3Client({
    region,
    credentials: { accessKeyId, secretAccessKey },
    ...(endpoint ? { endpoint, forcePathStyle: true } : {}),
  });
}

export async function POST(request: NextRequest) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const bucket = process.env.S3_BUCKET;
  if (!bucket) {
    return NextResponse.json(
      { error: "File storage not configured" },
      { status: 500 }
    );
  }

  try {
    const formData = await request.formData();
    const files = formData.getAll("files") as File[];

    if (!files || files.length === 0) {
      return NextResponse.json({ error: "No files provided" }, { status: 400 });
    }

    // Validate total size
    let totalSize = 0;
    for (const file of files) {
      totalSize += file.size;
    }

    if (totalSize > MAX_TOTAL_SIZE) {
      return NextResponse.json(
        { error: `Total file size exceeds ${MAX_TOTAL_SIZE / 1024 / 1024}MB limit` },
        { status: 400 }
      );
    }

    const s3 = getS3Client();
    const uploadedFiles: Array<{
      id: string;
      filename: string;
      content_type: string;
      size: number;
      url: string;
    }> = [];

    for (const file of files) {
      // Validate individual file size
      if (file.size > MAX_FILE_SIZE) {
        return NextResponse.json(
          { error: `File "${file.name}" exceeds ${MAX_FILE_SIZE / 1024 / 1024}MB limit` },
          { status: 400 }
        );
      }

      const fileId = randomUUID();
      const key = `attachments/${user.org_id}/${user.id}/${fileId}/${file.name}`;

      const buffer = Buffer.from(await file.arrayBuffer());

      await s3.send(
        new PutObjectCommand({
          Bucket: bucket,
          Key: key,
          Body: buffer,
          ContentType: file.type || "application/octet-stream",
          Metadata: {
            originalName: file.name,
            userId: user.id,
            orgId: user.org_id,
          },
        })
      );

      // Construct the URL
      const endpoint = process.env.S3_ENDPOINT;
      const publicUrl = process.env.S3_PUBLIC_URL;
      let url: string;

      if (publicUrl) {
        url = `${publicUrl}/${key}`;
      } else if (endpoint) {
        url = `${endpoint}/${bucket}/${key}`;
      } else {
        url = `https://${bucket}.s3.${process.env.AWS_REGION || "us-east-1"}.amazonaws.com/${key}`;
      }

      uploadedFiles.push({
        id: fileId,
        filename: file.name,
        content_type: file.type || "application/octet-stream",
        size: file.size,
        url,
      });
    }

    return NextResponse.json({ files: uploadedFiles }, { status: 201 });
  } catch (error) {
    console.error("Upload error:", error);
    if (error instanceof Error && error.message.includes("not configured")) {
      return NextResponse.json({ error: error.message }, { status: 500 });
    }
    return NextResponse.json(
      { error: "Failed to upload files" },
      { status: 500 }
    );
  }
}
