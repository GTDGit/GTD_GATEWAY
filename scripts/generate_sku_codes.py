#!/usr/bin/env python3
"""
Generate migration to rename sku_codes from long strings (ALT-xxx, KB-xxx)
to short 7-digit numeric format: CCBBSSS
  CC  = 2-digit category code (01-19)
  BB  = 2-digit brand code (01-99 per category)
  SSS = 3-digit sequence (001-999 per brand, sorted by denomination/name)
"""

import re
import os
import json
from collections import defaultdict

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_DIR = os.path.dirname(SCRIPT_DIR)
MIGRATIONS_DIR = os.path.join(PROJECT_DIR, "migrations")

# Fixed category ordering (user-facing priority)
CATEGORY_ORDER = [
    "Pulsa",               # 01
    "Paket Data",          # 02
    "Listrik",             # 03
    "E-Money",             # 04
    "Voucher Game",        # 05
    "Voucher",             # 06
    "TV Kabel",            # 07
    "Streaming",           # 08
    "Internet",            # 09
    "BPJS Kesehatan",      # 10
    "Telepon Pascabayar",  # 11
    "Gas PGN",             # 12
    "PDAM",                # 13
    "PBB",                 # 14
    "Multifinance",        # 15
    "Edukasi",             # 16
    "Donasi",              # 17
    "Properti",            # 18
    "Tiket",               # 19
]


def parse_products_from_migration(filepath):
    """Parse INSERT INTO products statements from migration SQL."""
    products = []
    with open(filepath, encoding="utf-8") as f:
        content = f.read()

    # Match: INSERT INTO products (sku_code, name, category, brand, type, admin, commission, description, is_active)
    # VALUES ('SKU', 'Name', 'Cat', 'Brand', 'type', admin, commission, 'desc', true/false)
    pattern = re.compile(
        r"INSERT INTO products\s*\([^)]+\)\s*VALUES\s*\("
        r"'([^']*(?:''[^']*)*)',\s*"   # sku_code (handles escaped quotes)
        r"'([^']*(?:''[^']*)*)',\s*"   # name
        r"'([^']*(?:''[^']*)*)',\s*"   # category
        r"'([^']*(?:''[^']*)*)',\s*"   # brand
        r"'(prepaid|postpaid)',\s*"     # type
        r"(\d+),\s*"                    # admin
        r"(\d+),\s*"                    # commission
        r"'([^']*(?:''[^']*)*)',\s*"   # description
        r"(true|false)"                 # is_active
    )

    for m in pattern.finditer(content):
        sku = m.group(1).replace("''", "'")
        name = m.group(2).replace("''", "'")
        category = m.group(3).replace("''", "'")
        brand = m.group(4).replace("''", "'")
        ptype = m.group(5)
        products.append({
            "sku": sku,
            "name": name,
            "category": category,
            "brand": brand,
            "type": ptype,
        })

    return products


def extract_denomination(name, sku):
    """Extract numeric denomination from product name or SKU for sorting."""
    # Try to find a number in the name (e.g., "Rp 50.000" -> 50000)
    # Remove dots/commas used as thousand separators
    nums = re.findall(r'[\d,.]+', name)
    for n in nums:
        cleaned = n.replace('.', '').replace(',', '')
        if cleaned.isdigit() and int(cleaned) > 0:
            return int(cleaned)

    # Try from SKU (e.g., ALT-PULSA-TELKOMSEL-50000)
    nums = re.findall(r'\d+', sku)
    for n in reversed(nums):  # last number is usually denomination
        if int(n) > 0:
            return int(n)

    return 0


def generate_sku_mapping(products):
    """Generate old_sku -> new_sku mapping."""
    # Group products by (category, brand)
    groups = defaultdict(list)
    for p in products:
        groups[(p["category"], p["brand"])].append(p)

    # Build category code map
    cat_codes = {}
    for i, cat in enumerate(CATEGORY_ORDER, 1):
        cat_codes[cat] = i

    # Check for unknown categories
    all_cats = set(p["category"] for p in products)
    unknown = all_cats - set(CATEGORY_ORDER)
    if unknown:
        print(f"WARNING: Unknown categories (will be appended): {unknown}")
        next_code = len(CATEGORY_ORDER) + 1
        for cat in sorted(unknown):
            cat_codes[cat] = next_code
            next_code += 1

    # Build brand code map per category
    brand_codes = {}  # (category, brand) -> code
    for cat in sorted(cat_codes.keys(), key=lambda c: cat_codes[c]):
        brands_in_cat = sorted(set(
            p["brand"] for p in products if p["category"] == cat
        ))
        for j, brand in enumerate(brands_in_cat, 1):
            if j > 99:
                raise ValueError(f"Category {cat} has {len(brands_in_cat)} brands (max 99)")
            brand_codes[(cat, brand)] = j

    # Sort products within each (category, brand) group and assign sequences
    mapping = {}  # old_sku -> new_sku
    for (cat, brand), prods in groups.items():
        cc = cat_codes[cat]
        bb = brand_codes[(cat, brand)]

        # Sort by denomination, then by name
        prods.sort(key=lambda p: (extract_denomination(p["name"], p["sku"]), p["name"]))

        for seq, p in enumerate(prods, 1):
            if seq > 999:
                raise ValueError(f"Brand {brand} in {cat} has {len(prods)} products (max 999)")
            new_sku = f"{cc:02d}{bb:02d}{seq:03d}"
            mapping[p["sku"]] = new_sku

    return mapping, cat_codes, brand_codes


def generate_migration_sql(mapping):
    """Generate UP and DOWN migration SQL."""
    up_lines = [
        "-- ============================================",
        "-- Migration 000030: Shorten SKU codes",
        "-- Auto-generated by scripts/generate_sku_codes.py",
        "-- Format: CCBBSSS (category + brand + sequence)",
        "-- ============================================",
        "",
        "BEGIN;",
        "",
    ]

    down_lines = [
        "-- ============================================",
        "-- Migration 000030: Revert SKU codes",
        "-- Auto-generated by scripts/generate_sku_codes.py",
        "-- ============================================",
        "",
        "BEGIN;",
        "",
    ]

    # Sort by new SKU for readable output
    sorted_items = sorted(mapping.items(), key=lambda x: x[1])

    for old_sku, new_sku in sorted_items:
        escaped_old = old_sku.replace("'", "''")
        up_lines.append(f"UPDATE products SET sku_code = '{new_sku}' WHERE sku_code = '{escaped_old}';")
        down_lines.append(f"UPDATE products SET sku_code = '{escaped_old}' WHERE sku_code = '{new_sku}';")

    up_lines.extend(["", "COMMIT;", ""])
    down_lines.extend(["", "COMMIT;", ""])

    return "\n".join(up_lines), "\n".join(down_lines)


def generate_reference(mapping, cat_codes, brand_codes, products):
    """Generate a human-readable reference of the coding scheme."""
    lines = ["# SKU Code Reference", "# Format: CCBBSSS", ""]

    # Category legend
    lines.append("## Categories")
    for cat, code in sorted(cat_codes.items(), key=lambda x: x[1]):
        count = sum(1 for p in products if p["category"] == cat)
        lines.append(f"  {code:02d} = {cat} ({count} products)")

    lines.append("")
    lines.append("## Brands per Category")

    # Brand legend per category
    for cat in sorted(cat_codes.keys(), key=lambda c: cat_codes[c]):
        cc = cat_codes[cat]
        brands = [(b, bc) for (c, b), bc in brand_codes.items() if c == cat]
        brands.sort(key=lambda x: x[1])
        lines.append(f"  Category {cc:02d} ({cat}):")
        for brand, bb in brands:
            count = sum(1 for p in products if p["category"] == cat and p["brand"] == brand)
            lines.append(f"    {bb:02d} = {brand} ({count} products)")

    return "\n".join(lines)


def main():
    # Parse products from both migrations
    print("Parsing migration 000028...")
    products_28 = parse_products_from_migration(
        os.path.join(MIGRATIONS_DIR, "000028_reseed_alterra_pricing.up.sql")
    )
    print(f"  Found {len(products_28)} products")

    print("Parsing migration 000029...")
    products_29 = parse_products_from_migration(
        os.path.join(MIGRATIONS_DIR, "000029_seed_kiosbank_products.up.sql")
    )
    print(f"  Found {len(products_29)} products")

    # Deduplicate by sku_code (migration 029 may reference same SKUs via ON CONFLICT)
    seen = set()
    all_products = []
    for p in products_28 + products_29:
        if p["sku"] not in seen:
            seen.add(p["sku"])
            all_products.append(p)

    print(f"Total unique products: {len(all_products)}")

    # Generate mapping
    mapping, cat_codes, brand_codes = generate_sku_mapping(all_products)
    print(f"Generated {len(mapping)} SKU mappings")

    # Verify no duplicates in new SKUs
    new_skus = list(mapping.values())
    if len(new_skus) != len(set(new_skus)):
        dupes = [s for s in new_skus if new_skus.count(s) > 1]
        raise ValueError(f"Duplicate new SKUs detected: {set(dupes)}")
    print("No duplicate new SKUs - OK")

    # Generate SQL
    up_sql, down_sql = generate_migration_sql(mapping)

    up_path = os.path.join(MIGRATIONS_DIR, "000030_shorten_sku_codes.up.sql")
    down_path = os.path.join(MIGRATIONS_DIR, "000030_shorten_sku_codes.down.sql")

    with open(up_path, "w", encoding="utf-8") as f:
        f.write(up_sql)
    print(f"Written: {up_path}")

    with open(down_path, "w", encoding="utf-8") as f:
        f.write(down_sql)
    print(f"Written: {down_path}")

    # Generate reference
    ref = generate_reference(mapping, cat_codes, brand_codes, all_products)
    ref_path = os.path.join(SCRIPT_DIR, "sku_code_reference.txt")
    with open(ref_path, "w", encoding="utf-8") as f:
        f.write(ref)
    print(f"Written: {ref_path}")

    # Generate JSON mapping
    json_path = os.path.join(SCRIPT_DIR, "sku_code_mapping.json")
    with open(json_path, "w", encoding="utf-8") as f:
        json.dump(mapping, f, indent=2, ensure_ascii=False)
    print(f"Written: {json_path}")

    # Print summary
    print("\n=== Summary ===")
    cats = defaultdict(int)
    for p in all_products:
        cats[p["category"]] += 1
    for cat in sorted(cats.keys(), key=lambda c: cat_codes.get(c, 99)):
        cc = cat_codes.get(cat, "??")
        print(f"  {cc:02d} {cat}: {cats[cat]} products")

    # Print a few examples
    print("\n=== Examples ===")
    examples = [
        "ALT-PULSA-TELKOMSEL-50000",
        "ALT-EMONEY-DANA-10000",
        "ALT-LISTRIK-PLN-TOKEN-20000",
        "KB-PDAM-PDAM-400011",
        "ALT-GAME-MOBILE LEGEND-86",
    ]
    for old in examples:
        if old in mapping:
            print(f"  {old} -> {mapping[old]}")


if __name__ == "__main__":
    main()
