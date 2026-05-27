#!/usr/bin/env bash
# Seed script: creates a merchant org + user, then populates products via the API.
# Usage: ./scripts/seed_products.sh
# Prerequisites: docker stack running (mise run composeup), API running (mise run run)

set -euo pipefail

BASE_URL="http://localhost:8080/v1"
IMAGE_DIR="$HOME/Pictures/Wallpapers/16x9"
DB_CONTAINER="ecommerce-db"
DB_NAME="ecommerce"
DB_USER="postgres"

psql_exec() {
  docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -t -A -q -c "$1"
}

echo "==> Step 1: Login as platform admin"
ADMIN_RESP=$(curl -sf -X POST "$BASE_URL/auth/admin/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@admin.com","password":"abcd1234"}')
ADMIN_TOKEN=$(echo "$ADMIN_RESP" | jq -r '.token')
echo "    Admin token: ${ADMIN_TOKEN:0:16}..."

echo "==> Step 2: Create merchant user"
MERCHANT_EMAIL="merchant@demo.com"
MERCHANT_PASSWORD="merchant1234"
MERCHANT_RESP=$(curl -sf -X POST "$BASE_URL/users/merchant" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"name\":\"Demo Merchant\",\"email\":\"$MERCHANT_EMAIL\",\"password\":\"$MERCHANT_PASSWORD\",\"roleSlug\":\"merchant.owner\"}" \
  || echo "")

if [ -z "$MERCHANT_RESP" ]; then
  echo "    Merchant user may already exist, continuing..."
fi

echo "==> Step 3: Resolve merchant user identity_id from DB"
IDENTITY_ID=$(psql_exec "SELECT i.id FROM identities i JOIN users u ON u.identity_id = i.id WHERE u.email = '$MERCHANT_EMAIL' LIMIT 1;")
if [ -z "$IDENTITY_ID" ]; then
  echo "ERROR: Could not find identity for $MERCHANT_EMAIL"
  exit 1
fi
echo "    identity_id: $IDENTITY_ID"

echo "==> Step 4: Create merchant organization (direct DB insert if not exists)"
EXISTING_ORG=$(psql_exec "SELECT id FROM organizations WHERE slug = 'demo-merchant' LIMIT 1;")
if [ -n "$EXISTING_ORG" ]; then
  ORG_ID="$EXISTING_ORG"
  echo "    Org already exists: $ORG_ID"
else
  ORG_ID=$(psql_exec "INSERT INTO organizations (name, slug, type, capability, status, metadata) VALUES ('Demo Merchant', 'demo-merchant', 'merchant', 'seller', 'active', '{}') RETURNING id;")
  echo "    Created org: $ORG_ID"
fi

echo "==> Step 5: Link identity to org as merchant.owner (if not already a member)"
EXISTING_MEMBER=$(psql_exec "SELECT id FROM members WHERE identity_id = '$IDENTITY_ID' AND organization_id = '$ORG_ID' LIMIT 1;")
if [ -n "$EXISTING_MEMBER" ]; then
  echo "    Member record already exists"
else
  MERCHANT_OWNER_ROLE_ID=$(psql_exec "SELECT id FROM roles WHERE slug = 'merchant.owner' AND organization_id IS NULL LIMIT 1;")
  MEMBER_ID=$(psql_exec "INSERT INTO members (identity_id, organization_id) VALUES ('$IDENTITY_ID', '$ORG_ID') RETURNING id;")
  psql_exec "INSERT INTO member_roles (member_id, role_id, assigned_by) VALUES ('$MEMBER_ID', $MERCHANT_OWNER_ROLE_ID, NULL);" > /dev/null
  echo "    Created member + role: member_id=$MEMBER_ID, role_id=$MERCHANT_OWNER_ROLE_ID"
fi

echo "==> Step 6: Login as merchant"
MERCH_RESP=$(curl -sf -X POST "$BASE_URL/auth/merchant/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$MERCHANT_EMAIL\",\"password\":\"$MERCHANT_PASSWORD\"}")
MERCH_TOKEN=$(echo "$MERCH_RESP" | jq -r '.token')
echo "    Merchant token: ${MERCH_TOKEN:0:16}..."

# ── Category and attribute value IDs ──────────────────────────────────────────
# Categories (leaf nodes used below):
CAT_CERAMIC_TILES=$(psql_exec "SELECT id FROM categories WHERE slug = 'ceramic-tiles' LIMIT 1;")
CAT_LAMINATE=$(psql_exec "SELECT id FROM categories WHERE slug = 'laminate-flooring' LIMIT 1;")
CAT_TIMBER_FLOORING=$(psql_exec "SELECT id FROM categories WHERE slug = 'timber-flooring' LIMIT 1;")
CAT_INTERIOR_PAINT=$(psql_exec "SELECT id FROM categories WHERE slug = 'interior-paint' LIMIT 1;")
CAT_WALL_TILES=$(psql_exec "SELECT id FROM categories WHERE slug = 'wall-tiles' LIMIT 1;")
CAT_LIGHTING=$(psql_exec "SELECT id FROM categories WHERE slug = 'lighting' LIMIT 1;")
CAT_WALLPAPER=$(psql_exec "SELECT id FROM categories WHERE slug = 'wallpaper' LIMIT 1;")
CAT_TIMBER_DOORS=$(psql_exec "SELECT id FROM categories WHERE slug = 'timber-doors' LIMIT 1;")

# Attribute value IDs:
AV_MAT_CERAMIC=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='material' AND av.value='ceramic' LIMIT 1;")
AV_MAT_TIMBER=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='material' AND av.value='timber' LIMIT 1;")
AV_MAT_VINYL=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='material' AND av.value='vinyl' LIMIT 1;")
AV_MAT_PORCELAIN=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='material' AND av.value='porcelain' LIMIT 1;")
AV_FINISH_MATTE=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='finish' AND av.value='matte' LIMIT 1;")
AV_FINISH_GLOSS=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='finish' AND av.value='gloss' LIMIT 1;")
AV_FINISH_SATIN=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='finish' AND av.value='satin' LIMIT 1;")
AV_FINISH_NATURAL=$(psql_exec "SELECT av.id FROM attribute_values av JOIN attributes a ON av.attribute_id = a.id WHERE a.slug='finish' AND av.value='natural' LIMIT 1;")

upload_asset() {
  local file_path="$1"
  local token
  local resp
  resp=$(curl -sf -X POST "$BASE_URL/product-assets" \
    -H "Authorization: Bearer $MERCH_TOKEN" \
    -F "files=@${file_path}")
  token=$(echo "$resp" | jq -r '.uploads[0].token')
  echo "$token"
}

create_product() {
  local name="$1"
  local slug="$2"
  local category_id="$3"
  local description="$4"
  local specification="$5"
  local asset_token="$6"
  local variants_json="$7"

  local payload
  payload=$(jq -n \
    --arg name "$name" \
    --arg slug "$slug" \
    --arg category_id "$category_id" \
    --argjson description "$description" \
    --argjson specification "$specification" \
    --arg token "$asset_token" \
    --argjson variants "$variants_json" \
    '{
      name: $name,
      slug: $slug,
      categoryId: $category_id,
      description: $description,
      specification: $specification,
      assets: [{ token: $token, isPrimary: true, sortOrder: 0 }],
      variants: $variants
    }')

  local resp
  resp=$(curl -sf -X POST "$BASE_URL/products" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $MERCH_TOKEN" \
    -d "$payload")

  local product_id
  product_id=$(echo "$resp" | jq -r '.product.id')
  echo "$product_id"
}

activate_product() {
  local product_id="$1"
  curl -sf -X PATCH "$BASE_URL/products/$product_id/status" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $MERCH_TOKEN" \
    -d '{"status":"active"}' > /dev/null
}

echo ""
echo "==> Step 7: Upload images and create products"
echo ""

# ── Product 1: Abstract Ceramic Floor Tiles ───────────────────────────────────
echo "  [1/8] Abstract Ceramic Floor Tiles"
TOKEN=$(upload_asset "$IMAGE_DIR/abstract-1.jpg")
PID=$(create_product \
  "Abstract Ceramic Floor Tiles" \
  "abstract-ceramic-floor-tiles" \
  "$CAT_CERAMIC_TILES" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Premium abstract-pattern ceramic floor tiles. Suitable for living areas, corridors, and commercial spaces. Easy to clean and highly durable."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Material: Ceramic | Finish: Matte | Size: 600×600 mm | Thickness: 10 mm | Water Absorption: <0.5%"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"ACT-600-MAT\",\"name\":\"600×600 Matte\",\"price\":\"45.00\",\"attributeValueIds\":[$AV_MAT_CERAMIC,$AV_FINISH_MATTE]},
    {\"sku\":\"ACT-600-GLO\",\"name\":\"600×600 Gloss\",\"price\":\"49.00\",\"attributeValueIds\":[$AV_MAT_CERAMIC,$AV_FINISH_GLOSS]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 2: Abstract Porcelain Wall Tiles ─────────────────────────────────
echo "  [2/8] Abstract Porcelain Wall Tiles"
TOKEN=$(upload_asset "$IMAGE_DIR/abstract-2.jpg")
PID=$(create_product \
  "Abstract Porcelain Wall Tiles" \
  "abstract-porcelain-wall-tiles" \
  "$CAT_WALL_TILES" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"High-definition porcelain wall tiles with a modern abstract pattern. Perfect for feature walls, bathrooms, and kitchens."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Material: Porcelain | Finish: Satin | Size: 300×600 mm | Thickness: 9 mm | Frost Resistant: Yes"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"APT-300-SAT\",\"name\":\"300×600 Satin\",\"price\":\"52.00\",\"attributeValueIds\":[$AV_MAT_PORCELAIN,$AV_FINISH_SATIN]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 3: Classic Timber Laminate Flooring ───────────────────────────────
echo "  [3/8] Classic Timber Laminate Flooring"
TOKEN=$(upload_asset "$IMAGE_DIR/landscape-1.jpg")
PID=$(create_product \
  "Classic Timber Laminate Flooring" \
  "classic-timber-laminate-flooring" \
  "$CAT_LAMINATE" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Warm and natural-looking laminate flooring with a timber grain finish. Easy click-lock installation, suitable for DIY and professional use."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Material: Laminate over HDF core | Finish: Natural | Plank Size: 1200×190 mm | Thickness: 12 mm | AC Rating: AC4 | Warranty: 25 years"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"TLF-1200-NAT\",\"name\":\"1200×190 Natural\",\"price\":\"38.00\",\"attributeValueIds\":[$AV_MAT_VINYL,$AV_FINISH_NATURAL]},
    {\"sku\":\"TLF-1200-MAT\",\"name\":\"1200×190 Matte\",\"price\":\"36.00\",\"attributeValueIds\":[$AV_MAT_VINYL,$AV_FINISH_MATTE]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 4: Engineered Timber Flooring ────────────────────────────────────
echo "  [4/8] Engineered Timber Flooring"
TOKEN=$(upload_asset "$IMAGE_DIR/landscape-2.jpg")
PID=$(create_product \
  "Engineered Timber Flooring" \
  "engineered-timber-flooring" \
  "$CAT_TIMBER_FLOORING" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Genuine engineered timber flooring with a hardwood veneer layer over a stable plywood base. Ideal for areas with slight moisture variation."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Material: Timber (Oak veneer over plywood) | Finish: Natural oil | Plank Size: 1800×190 mm | Thickness: 14 mm | Can be sanded: 2× | Suitable for underfloor heating: Yes"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"ETF-1800-NAT\",\"name\":\"1800×190 Natural\",\"price\":\"89.00\",\"attributeValueIds\":[$AV_MAT_TIMBER,$AV_FINISH_NATURAL]},
    {\"sku\":\"ETF-1800-MAT\",\"name\":\"1800×190 Matte\",\"price\":\"85.00\",\"attributeValueIds\":[$AV_MAT_TIMBER,$AV_FINISH_MATTE]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 5: Premium Interior Wall Paint ───────────────────────────────────
echo "  [5/8] Premium Interior Wall Paint"
TOKEN=$(upload_asset "$IMAGE_DIR/landscape-3.jpg")
PID=$(create_product \
  "Premium Interior Wall Paint" \
  "premium-interior-wall-paint" \
  "$CAT_INTERIOR_PAINT" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"A premium low-VOC interior wall paint with excellent coverage and a smooth, washable finish. Available in three sheens: Matte, Satin, and Semi-Gloss."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Coverage: 10–12 m²/L | Dry time: 1 hour (recoat 4 hours) | VOC: <5 g/L | Finish options: Matte, Satin, Semi-Gloss | Container: 4 L / 15 L"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"IWP-4L-MAT\",\"name\":\"4 L Matte\",\"price\":\"55.00\",\"attributeValueIds\":[$AV_FINISH_MATTE]},
    {\"sku\":\"IWP-4L-SAT\",\"name\":\"4 L Satin\",\"price\":\"58.00\",\"attributeValueIds\":[$AV_FINISH_SATIN]},
    {\"sku\":\"IWP-15L-MAT\",\"name\":\"15 L Matte\",\"price\":\"185.00\",\"attributeValueIds\":[$AV_FINISH_MATTE]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 6: Designer Wallpaper ─────────────────────────────────────────────
echo "  [6/8] Designer Wallpaper"
TOKEN=$(upload_asset "$IMAGE_DIR/cyberpunk-1.jpg")
PID=$(create_product \
  "Designer Wallpaper — Urban Series" \
  "designer-wallpaper-urban-series" \
  "$CAT_WALLPAPER" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Bold, statement-making wallpaper for feature walls. Printed on a vinyl-coated paper substrate — moisture resistant and easy to paste and clean."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Material: Vinyl-coated paper | Roll size: 0.53 m × 10 m | Coverage per roll: ~5 m² | Repeat: 64 cm | Paste type: Ready-mixed"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"WPP-URB-001\",\"name\":\"Urban Series — Roll\",\"price\":\"32.00\",\"attributeValueIds\":[$AV_FINISH_MATTE]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 7: Solid Timber Door ──────────────────────────────────────────────
echo "  [7/8] Solid Timber Door"
TOKEN=$(upload_asset "$IMAGE_DIR/cars-1.jpg")
PID=$(create_product \
  "Solid Timber Interior Door" \
  "solid-timber-interior-door" \
  "$CAT_TIMBER_DOORS" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Solid hardwood interior door with a classic panelled design. Pre-primed for easy finishing. Sold as slab only — frame and hardware not included."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Material: Solid timber (Meranti) | Finish: Pre-primed | Standard size: 2040×820×40 mm | Custom sizes available | Pre-hung: No"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"STD-820-NAT\",\"name\":\"820 mm Natural\",\"price\":\"290.00\",\"attributeValueIds\":[$AV_MAT_TIMBER,$AV_FINISH_NATURAL]},
    {\"sku\":\"STD-920-NAT\",\"name\":\"920 mm Natural\",\"price\":\"310.00\",\"attributeValueIds\":[$AV_MAT_TIMBER,$AV_FINISH_NATURAL]}
  ]")
activate_product "$PID"
echo "     id=$PID"

# ── Product 8: LED Pendant Light ──────────────────────────────────────────────
echo "  [8/8] LED Pendant Light"
TOKEN=$(upload_asset "$IMAGE_DIR/abstract-1.jpg")
PID=$(create_product \
  "LED Pendant Light — Modern Series" \
  "led-pendant-light-modern-series" \
  "$CAT_LIGHTING" \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Minimalist LED pendant light suitable for kitchens, dining rooms, and living areas. Adjustable cord length. Compatible with standard E27 fittings."}]}]}' \
  '{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Wattage: 12 W LED (equivalent 75 W) | Colour temp: 3000 K warm white | IP Rating: IP20 (indoor only) | Cord length: 0.5–1.5 m adjustable | Material: Aluminium + Glass shade"}]}]}' \
  "$TOKEN" \
  "[
    {\"sku\":\"LPL-MOD-BLK\",\"name\":\"Black\",\"price\":\"149.00\",\"attributeValueIds\":[$AV_MAT_PORCELAIN,$AV_FINISH_MATTE]},
    {\"sku\":\"LPL-MOD-WHT\",\"name\":\"White\",\"price\":\"149.00\",\"attributeValueIds\":[$AV_MAT_PORCELAIN,$AV_FINISH_GLOSS]}
  ]")
activate_product "$PID"
echo "     id=$PID"

echo ""
echo "==> All done! 8 products created and activated."
echo "    Merchant org id : $ORG_ID"
echo "    Storefront test : curl '$BASE_URL/products' | jq '.'"
