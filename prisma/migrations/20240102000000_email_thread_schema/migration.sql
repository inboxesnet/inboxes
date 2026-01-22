-- CreateEnum
CREATE TYPE "Folder" AS ENUM ('inbox', 'sent', 'archive', 'trash');

-- CreateEnum
CREATE TYPE "EmailDirection" AS ENUM ('inbound', 'outbound');

-- CreateEnum
CREATE TYPE "EmailStatus" AS ENUM ('received', 'sent', 'delivered', 'bounced', 'failed');

-- CreateTable
CREATE TABLE "Thread" (
    "id" TEXT NOT NULL,
    "org_id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "subject" TEXT NOT NULL,
    "participant_emails" JSONB NOT NULL DEFAULT '[]',
    "last_message_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "message_count" INTEGER NOT NULL DEFAULT 0,
    "unread_count" INTEGER NOT NULL DEFAULT 0,
    "starred" BOOLEAN NOT NULL DEFAULT false,
    "folder" "Folder" NOT NULL DEFAULT 'inbox',
    "deleted_at" TIMESTAMP(3),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Thread_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Email" (
    "id" TEXT NOT NULL,
    "org_id" TEXT NOT NULL,
    "thread_id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "message_id" TEXT,
    "in_reply_to" TEXT,
    "references_header" JSONB,
    "from_address" TEXT NOT NULL,
    "to_addresses" JSONB NOT NULL DEFAULT '[]',
    "cc_addresses" JSONB NOT NULL DEFAULT '[]',
    "bcc_addresses" JSONB NOT NULL DEFAULT '[]',
    "subject" TEXT NOT NULL,
    "body_html" TEXT NOT NULL,
    "body_plain" TEXT NOT NULL,
    "attachments" JSONB NOT NULL DEFAULT '[]',
    "direction" "EmailDirection" NOT NULL,
    "status" "EmailStatus" NOT NULL,
    "read" BOOLEAN NOT NULL DEFAULT false,
    "starred" BOOLEAN NOT NULL DEFAULT false,
    "folder" "Folder" NOT NULL DEFAULT 'inbox',
    "deleted_at" TIMESTAMP(3),
    "trash_expires_at" TIMESTAMP(3),
    "delivered_via_alias" TEXT,
    "original_to" TEXT,
    "received_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "Email_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "Thread_org_id_idx" ON "Thread"("org_id");

-- CreateIndex
CREATE INDEX "Thread_user_id_idx" ON "Thread"("user_id");

-- CreateIndex
CREATE INDEX "Thread_folder_idx" ON "Thread"("folder");

-- CreateIndex
CREATE INDEX "Thread_last_message_at_idx" ON "Thread"("last_message_at");

-- CreateIndex
CREATE INDEX "Email_user_id_idx" ON "Email"("user_id");

-- CreateIndex
CREATE INDEX "Email_thread_id_idx" ON "Email"("thread_id");

-- CreateIndex
CREATE INDEX "Email_org_id_idx" ON "Email"("org_id");

-- CreateIndex
CREATE INDEX "Email_message_id_idx" ON "Email"("message_id");

-- CreateIndex
CREATE INDEX "Email_folder_idx" ON "Email"("folder");

-- CreateIndex
CREATE INDEX "Email_received_at_idx" ON "Email"("received_at");

-- AddForeignKey
ALTER TABLE "Thread" ADD CONSTRAINT "Thread_org_id_fkey" FOREIGN KEY ("org_id") REFERENCES "Org"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Thread" ADD CONSTRAINT "Thread_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "User"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Email" ADD CONSTRAINT "Email_org_id_fkey" FOREIGN KEY ("org_id") REFERENCES "Org"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Email" ADD CONSTRAINT "Email_thread_id_fkey" FOREIGN KEY ("thread_id") REFERENCES "Thread"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Email" ADD CONSTRAINT "Email_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "User"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
