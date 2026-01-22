-- AlterTable
ALTER TABLE "User" ADD COLUMN "reset_token" TEXT;
ALTER TABLE "User" ADD COLUMN "reset_token_expires_at" TIMESTAMP(3);
