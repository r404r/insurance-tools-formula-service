#!/usr/bin/env python3
"""
缓存机制验证测试
每组 6 次调用：前 3 次入参不同（预期 cache miss），
后 3 次其中 2 次复用前面的入参（预期 cache hit），1 次全新入参（预期 cache miss）。
通过响应中的 cacheHit 字段 + executionTimeMs 双维度判断缓存是否生效。
"""

import json
import time
import urllib.request
import urllib.error
import sys
from datetime import datetime

BASE = "http://localhost:8080/api/v1"
ADMIN_USER = "admin"
ADMIN_PASS = "admin99999"

# ── helpers ──────────────────────────────────────────────────────────────────

def post(path, body, token=None):
    data = json.dumps(body).encode()
    req = urllib.request.Request(
        BASE + path,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    t0 = time.perf_counter()
    with urllib.request.urlopen(req) as resp:
        elapsed_ms = (time.perf_counter() - t0) * 1000
        return json.loads(resp.read()), elapsed_ms

def delete(path, token):
    req = urllib.request.Request(BASE + path, method="DELETE")
    req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req) as resp:
        return resp.status

def get(path, token):
    req = urllib.request.Request(BASE + path)
    req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())

def calc(token, formula_id, inputs):
    body = {"formulaId": formula_id, "inputs": inputs}
    return post("/calculate", body, token)

# ── test cases ───────────────────────────────────────────────────────────────

CASES = [
    {
        "name": "Case A — 日本生命保険 純保険料（deathBenefit / expectedDeaths / policyCount）",
        "formula_id": "d19c8b78-8423-4297-8386-b13adaf7846d",
        "calls": [
            # call 1-3: 不同入参 → cache miss
            {"tag": "1 [新入参]", "inputs": {"deathBenefit": "1000000", "expectedDeaths": "5",  "policyCount": "1000"}, "expect_hit": False},
            {"tag": "2 [新入参]", "inputs": {"deathBenefit": "2000000", "expectedDeaths": "8",  "policyCount": "500"},  "expect_hit": False},
            {"tag": "3 [新入参]", "inputs": {"deathBenefit": "500000",  "expectedDeaths": "3",  "policyCount": "2000"}, "expect_hit": False},
            # call 4-6: 2次复用 (命中), 1次新入参
            {"tag": "4 [重复 #1]", "inputs": {"deathBenefit": "1000000", "expectedDeaths": "5",  "policyCount": "1000"}, "expect_hit": True},
            {"tag": "5 [重复 #2]", "inputs": {"deathBenefit": "2000000", "expectedDeaths": "8",  "policyCount": "500"},  "expect_hit": True},
            {"tag": "6 [新入参]", "inputs": {"deathBenefit": "750000",  "expectedDeaths": "10", "policyCount": "800"},  "expect_hit": False},
        ],
    },
    {
        "name": "Case B — 寿险净保费计算（sumAssured / qx / interestRate）",
        "formula_id": "d764dfd9-777a-43e6-956e-8fbcb467db6f",
        "calls": [
            {"tag": "1 [新入参]", "inputs": {"sumAssured": "100000", "qx": "0.005",  "interestRate": "0.03"}, "expect_hit": False},
            {"tag": "2 [新入参]", "inputs": {"sumAssured": "200000", "qx": "0.008",  "interestRate": "0.04"}, "expect_hit": False},
            {"tag": "3 [新入参]", "inputs": {"sumAssured": "500000", "qx": "0.012",  "interestRate": "0.05"}, "expect_hit": False},
            {"tag": "4 [重复 #3]", "inputs": {"sumAssured": "500000", "qx": "0.012",  "interestRate": "0.05"}, "expect_hit": True},
            {"tag": "5 [重复 #1]", "inputs": {"sumAssured": "100000", "qx": "0.005",  "interestRate": "0.03"}, "expect_hit": True},
            {"tag": "6 [新入参]", "inputs": {"sumAssured": "300000", "qx": "0.007",  "interestRate": "0.035"},"expect_hit": False},
        ],
    },
    {
        "name": "Case C — 财产险保费计算（baseRate / riskScore / sumInsured / discount）",
        "formula_id": "06b6a395-92a2-42a2-8f12-6605e2e2f7ac",
        "calls": [
            {"tag": "1 [新入参]", "inputs": {"baseRate": "0.02", "riskScore": "1.0", "sumInsured": "500000",  "discount": "0.9"},  "expect_hit": False},
            {"tag": "2 [新入参]", "inputs": {"baseRate": "0.03", "riskScore": "1.5", "sumInsured": "1000000", "discount": "0.85"}, "expect_hit": False},
            {"tag": "3 [新入参]", "inputs": {"baseRate": "0.015","riskScore": "0.8", "sumInsured": "300000",  "discount": "0.95"}, "expect_hit": False},
            {"tag": "4 [重复 #2]", "inputs": {"baseRate": "0.03", "riskScore": "1.5", "sumInsured": "1000000", "discount": "0.85"}, "expect_hit": True},
            {"tag": "5 [重复 #3]", "inputs": {"baseRate": "0.015","riskScore": "0.8", "sumInsured": "300000",  "discount": "0.95"}, "expect_hit": True},
            {"tag": "6 [新入参]", "inputs": {"baseRate": "0.025","riskScore": "1.2", "sumInsured": "750000",  "discount": "0.88"}, "expect_hit": False},
        ],
    },
    {
        "name": "Case D — 车险商业保费计算（basePremium / vehicleFactor / driverFactor / ncdDiscount）",
        "formula_id": "da381e5e-34d7-414f-bb17-f21c92b1541e",
        "calls": [
            {"tag": "1 [新入参]", "inputs": {"basePremium": "3000", "vehicleFactor": "1.2", "driverFactor": "1.0", "ncdDiscount": "0.8"},  "expect_hit": False},
            {"tag": "2 [新入参]", "inputs": {"basePremium": "5000", "vehicleFactor": "1.5", "driverFactor": "1.3", "ncdDiscount": "0.7"},  "expect_hit": False},
            {"tag": "3 [新入参]", "inputs": {"basePremium": "2000", "vehicleFactor": "0.9", "driverFactor": "0.8", "ncdDiscount": "0.95"}, "expect_hit": False},
            {"tag": "4 [重复 #1]", "inputs": {"basePremium": "3000", "vehicleFactor": "1.2", "driverFactor": "1.0", "ncdDiscount": "0.8"},  "expect_hit": True},
            {"tag": "5 [新入参]", "inputs": {"basePremium": "4000", "vehicleFactor": "1.1", "driverFactor": "1.2", "ncdDiscount": "0.75"}, "expect_hit": False},
            {"tag": "6 [重复 #3]", "inputs": {"basePremium": "2000", "vehicleFactor": "0.9", "driverFactor": "0.8", "ncdDiscount": "0.95"}, "expect_hit": True},
        ],
    },
]

# ── runner ───────────────────────────────────────────────────────────────────

def run():
    print(f"\n{'='*72}")
    print("  缓存机制验证测试报告")
    print(f"  时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"{'='*72}\n")

    token, _ = post("/auth/login", {"username": ADMIN_USER, "password": ADMIN_PASS})
    token = token["token"]

    # 清空缓存，确保从干净状态开始
    delete("/cache", token)
    cache_before = get("/cache", token)
    print(f"✓ 缓存已清空  当前条目数: {cache_before['size']} / {cache_before['maxSize']}\n")

    total_calls = 0
    total_pass  = 0
    case_summaries = []

    for case in CASES:
        print(f"{'─'*72}")
        print(f"  {case['name']}")
        print(f"{'─'*72}")
        print(f"  {'调用':>2}  {'标签':<14}  {'结果':>14}  {'耗时(ms)':>9}  {'cacheHit':>9}  {'预期':>8}  {'判定':>6}")
        print(f"  {'─'*2}  {'─'*14}  {'─'*14}  {'─'*9}  {'─'*9}  {'─'*8}  {'─'*6}")

        miss_times = []
        hit_times  = []
        call_results = []

        for i, call in enumerate(case["calls"], 1):
            resp, wall_ms = calc(token, case["formula_id"], call["inputs"])
            actual_hit    = resp["cacheHit"]
            engine_ms     = resp["executionTimeMs"]
            result_val    = list(resp["result"].values())[0] if resp["result"] else "—"
            expect_hit    = call["expect_hit"]
            passed        = (actual_hit == expect_hit)

            verdict = "✓ PASS" if passed else "✗ FAIL"
            print(f"  {i:>2}  {call['tag']:<14}  {result_val:>14}  "
                  f"{engine_ms:>8.3f}  {'HIT' if actual_hit else 'MISS':>9}  "
                  f"{'HIT' if expect_hit else 'MISS':>8}  {verdict:>6}")

            if actual_hit:
                hit_times.append(engine_ms)
            else:
                miss_times.append(engine_ms)

            call_results.append(passed)
            total_calls += 1
            if passed:
                total_pass += 1

        # case stats
        avg_miss = sum(miss_times) / len(miss_times) if miss_times else 0
        avg_hit  = sum(hit_times)  / len(hit_times)  if hit_times  else 0
        speedup  = avg_miss / avg_hit if avg_hit > 0 else float("inf")
        case_pass = all(call_results)

        print()
        print(f"  平均耗时  → MISS: {avg_miss:.3f} ms   HIT: {avg_hit:.3f} ms   "
              f"加速比: {speedup:.1f}x")
        print(f"  Case 判定: {'全部通过 ✓' if case_pass else '存在失败 ✗'}")
        print()

        case_summaries.append({
            "name":     case["name"],
            "pass":     case_pass,
            "avg_miss": avg_miss,
            "avg_hit":  avg_hit,
            "speedup":  speedup,
        })

    # ── final report ─────────────────────────────────────────────────────────
    cache_after = get("/cache", token)

    print(f"{'='*72}")
    print("  最终测试报告")
    print(f"{'='*72}")
    print(f"  总调用次数:  {total_calls}")
    print(f"  通过 / 失败: {total_pass} / {total_calls - total_pass}")
    print(f"  通过率:      {total_pass/total_calls*100:.1f}%")
    print(f"  缓存条目数:  {cache_after['size']} / {cache_after['maxSize']}")
    print()
    print(f"  {'Case':<55}  {'结果':>6}  {'MISS(ms)':>9}  {'HIT(ms)':>8}  {'加速比':>7}")
    print(f"  {'─'*55}  {'─'*6}  {'─'*9}  {'─'*8}  {'─'*7}")
    for s in case_summaries:
        name = s["name"][:55]
        verdict = "PASS ✓" if s["pass"] else "FAIL ✗"
        sp = f"{s['speedup']:.1f}x" if s["speedup"] != float("inf") else "∞"
        print(f"  {name:<55}  {verdict:>6}  {s['avg_miss']:>9.3f}  {s['avg_hit']:>8.3f}  {sp:>7}")
    print()

    all_passed = all(s["pass"] for s in case_summaries)
    if all_passed:
        print("  ✓ 缓存机制验证通过：命中/未命中行为完全符合预期。")
    else:
        print("  ✗ 存在失败用例，请检查上方详细输出。")
    print(f"{'='*72}\n")

    return 0 if all_passed else 1

if __name__ == "__main__":
    sys.exit(run())
