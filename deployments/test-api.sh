#!/bin/bash
# Test all API endpoints
set -e

echo "===== Login ====="
LOGIN=$(curl -s -X POST http://localhost:8088/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}')
echo "$LOGIN" | head -c 200
echo ""
TOKEN=$(echo "$LOGIN" | grep -o '"token":"[^"]*"' | sed 's/"token":"//;s/"//')
echo "Token: ${TOKEN:0:50}..."
echo ""

AUTH="Authorization: Bearer $TOKEN"

for ep in \
    "/dashboard/overview" \
    "/dashboard/sales-trend?days=30" \
    "/dashboard/category-share" \
    "/dashboard/product/stats" \
    "/dashboard/profit/stats" \
    "/dashboard/ai/stats" \
    "/products?page=1&page_size=5" \
    "/products/1" \
    "/products/1/trends" \
    "/products/1/competitors" \
    "/purchases/orders?page=1&page_size=5" \
    "/inventory?page=1&page_size=5" \
    "/inventory/alerts" \
    "/finance/bills?page=1&page_size=5" \
    "/finance/profit/summary" \
    "/finance/profit/by-sku" \
    "/ai/workflows?page=1&page_size=20" \
    "/suppliers?page=1&page_size=5" \
    "/platform/accounts"; do
    echo "===== $ep ====="
    curl -s -H "$AUTH" "http://localhost:8088/api/v1$ep" | head -c 400
    echo ""
done
