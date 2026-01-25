-- Add notification_preferences JSON column to User table
ALTER TABLE "User" ADD COLUMN "notification_preferences" JSONB NOT NULL DEFAULT '{"browser_notifications": true, "notification_sound": false}';
