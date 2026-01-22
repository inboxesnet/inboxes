-- CreateTable
CREATE TABLE "Alias" (
    "id" TEXT NOT NULL,
    "org_id" TEXT NOT NULL,
    "address" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Alias_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "AliasUser" (
    "id" TEXT NOT NULL,
    "alias_id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "can_send_as" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "AliasUser_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "Alias_address_key" ON "Alias"("address");

-- CreateIndex
CREATE INDEX "Alias_org_id_idx" ON "Alias"("org_id");

-- CreateIndex
CREATE INDEX "AliasUser_alias_id_idx" ON "AliasUser"("alias_id");

-- CreateIndex
CREATE INDEX "AliasUser_user_id_idx" ON "AliasUser"("user_id");

-- CreateIndex
CREATE UNIQUE INDEX "AliasUser_alias_id_user_id_key" ON "AliasUser"("alias_id", "user_id");

-- AddForeignKey
ALTER TABLE "Alias" ADD CONSTRAINT "Alias_org_id_fkey" FOREIGN KEY ("org_id") REFERENCES "Org"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "AliasUser" ADD CONSTRAINT "AliasUser_alias_id_fkey" FOREIGN KEY ("alias_id") REFERENCES "Alias"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "AliasUser" ADD CONSTRAINT "AliasUser_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "User"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Email" ADD CONSTRAINT "Email_delivered_via_alias_fkey" FOREIGN KEY ("delivered_via_alias") REFERENCES "Alias"("id") ON DELETE SET NULL ON UPDATE CASCADE;
