#!/usr/bin/env python3
"""
Analyze and compare API reliability stress test results
"""

import argparse
import json
import os
import glob
from pathlib import Path

def analyze_session(session_dir):
    """Analyze a single test session"""
    if not os.path.exists(session_dir):
        return None
    
    analysis = {
        'session_dir': session_dir,
        'total_records': 0,
        'complete': 0,
        'incomplete': 0,
        'empty': 0,
        'caught_incomplete': 0,
        'caught_refusal': 0,
        'caught_missing_finish': 0,
        'sample_incomplete': []
    }
    
    # Analyze dataset
    dataset_path = os.path.join(session_dir, 'dataset.jsonl')
    if os.path.exists(dataset_path):
        with open(dataset_path, 'r') as f:
            for line in f:
                try:
                    record = json.loads(line)
                    output_text = None

                    # Newer SFT/DPO formats use a conversations array
                    if 'output' in record:
                        output_text = str(record['output']).strip()
                    elif isinstance(record.get('conversations'), list) and record['conversations']:
                        last_turn = record['conversations'][-1]
                        if isinstance(last_turn, dict) and 'value' in last_turn:
                            output_text = str(last_turn['value']).strip()

                    if output_text is None:
                        continue

                    analysis['total_records'] += 1
                    
                    if not output_text:
                        analysis['empty'] += 1
                    elif output_text[-1] in '.!?"':
                        analysis['complete'] += 1
                    else:
                        analysis['incomplete'] += 1
                        if len(analysis['sample_incomplete']) < 3:
                            analysis['sample_incomplete'].append({
                                'length': len(output_text),
                                'ending': output_text[-60:]
                            })
                except:
                    pass
    
    # Analyze logs
    log_path = os.path.join(session_dir, 'session.log')
    if os.path.exists(log_path):
        with open(log_path, 'r') as f:
            for line in f:
                try:
                    entry = json.loads(line)
                    if entry.get('level') in ['WARN', 'ERROR']:
                        msg = entry.get('msg', '')
                        if 'Incomplete output detected' in msg:
                            analysis['caught_incomplete'] += 1
                        elif 'Invalid finish_reason' in msg:
                            analysis['caught_missing_finish'] += 1
                        elif 'refused' in msg.lower() and 'Chosen response' in msg:
                            analysis['caught_refusal'] += 1

                    # Capture the high-level pipeline summary if present
                    if entry.get('msg') == 'Generation pipeline completed':
                        analysis['log_total_prompts'] = entry.get('total_prompts', 0)
                        analysis['log_successful'] = entry.get('successful', 0)
                        analysis['log_failed'] = entry.get('failed', 0)
                except:
                    pass
    
    return analysis


def find_latest_sessions(base_dir='output', max_sessions=2):
    """Find the most recent stress test sessions (heuristic: >=90 jobs).

    This is primarily used for the "no-args" mode so that
    run_test.sh can keep working as before. For precise analysis of
    a specific run, prefer passing the session directory explicitly
    on the command line.
    """
    sessions = []
    if os.path.exists(base_dir):
        for session_dir in sorted(glob.glob(os.path.join(base_dir, 'session_*')), reverse=True):
            # Check if it's a stress test (look for 90+ jobs in logs)
            log_path = os.path.join(session_dir, 'session.log')
            if os.path.exists(log_path):
                with open(log_path, 'r') as f:
                    job_count = 0
                    for line in f:
                        if 'Job processing breakdown' in line or 'Job failed' in line:
                            job_count += 1
                            if job_count >= 90:  # Likely a stress test
                                sessions.append(session_dir)
                                break
            if len(sessions) >= max_sessions:
                break

    return sessions


def main():
    parser = argparse.ArgumentParser(description="Analyze API reliability stress test results")
    parser.add_argument(
        "sessions",
        nargs="*",
        help=(
            "Specific session directories to analyze (e.g. "
            "output/session_2025-11-17T13-14-06). If omitted, the script "
            "auto-detects the most recent stress-test sessions (as before)."
        ),
    )
    parser.add_argument(
        "--max-auto",
        type=int,
        default=2,
        help="Maximum number of auto-detected sessions when no explicit sessions are provided (default: 2)",
    )
    parser.add_argument(
        "--base-dir",
        default="output",
        help="Base output directory where session_* folders are stored (default: output)",
    )

    args = parser.parse_args()

    print("=" * 80)
    print("API RELIABILITY STRESS TEST - ANALYSIS")
    print("=" * 80)
    print()

    # Determine which sessions to analyze
    if args.sessions:
        sessions = args.sessions
    else:
        sessions = find_latest_sessions(base_dir=args.base_dir, max_sessions=args.max_auto)

    if not sessions:
        print("⚠️  Could not find any recent test sessions.")
        print("   Run the stress tests first with: ./tests/api-reliability-test/run_test.sh")
        return

    # Analyze each session
    analyses = []
    for session_dir in sessions:
        if not os.path.exists(session_dir):
            print(f"⚠️  Skipping missing session directory: {session_dir}")
            continue

        # Determine provider from config snapshot if possible
        config_path = os.path.join(session_dir, "config.toml.bak")
        provider_name = "unknown"
        if os.path.exists(config_path):
            with open(config_path, "r") as f:
                content = f.read()
                if "nahcrof" in content:
                    provider_name = "nahcrof"
                elif "chutes" in content:
                    provider_name = "chutes"

        analysis = analyze_session(session_dir)
        if analysis:
            analysis["provider"] = provider_name
            analyses.append(analysis)

    if not analyses:
        print("⚠️  No analyzable sessions found.")
        return

    # Two modes:
    # 1) Explicit sessions provided → per-session report (no provider aggregation)
    # 2) No sessions provided → maintain old provider-comparison behavior

    if args.sessions:
        # Per-session reporting
        print("📊 SESSION ANALYSIS")
        print("=" * 80)
        print()

        for r in analyses:
            total_attempts = (
                r["total_records"]
                + r["caught_incomplete"]
                + r["caught_refusal"]
                + r["caught_missing_finish"]
            )
            success_rate = (
                r["total_records"] / total_attempts * 100 if total_attempts > 0 else 0
            )

            provider = r.get("provider", "unknown")
            print(f"Session: {os.path.basename(r['session_dir'])}")
            print(f"  Provider: {provider.upper()}")
            print(f"  Total attempts: {total_attempts}")
            print(f"  ✓ Succeeded: {r['total_records']} ({success_rate:.1f}%)")
            print(f"  ✗ Failed: {total_attempts - r['total_records']} ({100-success_rate:.1f}%)")
            print()
            print("  Failure breakdown:")
            print(f"    Incomplete outputs caught: {r['caught_incomplete']}")
            print(f"    Missing finish_reason: {r['caught_missing_finish']}")
            print(f"    Refusals: {r['caught_refusal']}")
            print()
            print("  Dataset quality:")
            print(f"    Total records: {r['total_records']}")
            print(
                f"    Complete: {r['complete']} "
                f"({r['complete']/r['total_records']*100 if r['total_records'] > 0 else 0:.1f}%)"
            )
            print(f"    Incomplete: {r['incomplete']}")
            print()

            if r["sample_incomplete"]:
                print("  Sample incomplete outputs saved:")
                for sample in r["sample_incomplete"]:
                    print(
                        f"    • {sample['length']:,} chars - ends: '...{sample['ending']}'"
                    )
                print()

            print("-" * 80)
            print()

        provider_groups = {}
        for r in analyses:
            provider_groups.setdefault(r["provider"], []).append(r)
    else:
        # Backwards-compatible provider comparison (used by run_test.sh)
        print("📊 PROVIDER COMPARISON")
        print("=" * 80)
        print()

        provider_results = {}
        for r in analyses:
            provider_results[r["provider"]] = r  # keep the latest per provider

        providers = (
            ["nahcrof", "chutes"]
            if "nahcrof" in provider_results and "chutes" in provider_results
            else list(provider_results.keys())
        )

        for provider in providers:
            if provider not in provider_results:
                continue

            r = provider_results[provider]
            total_attempts = (
                r["total_records"]
                + r["caught_incomplete"]
                + r["caught_refusal"]
                + r["caught_missing_finish"]
            )
            success_rate = (
                r["total_records"] / total_attempts * 100 if total_attempts > 0 else 0
            )

            print(f"Provider: {provider.upper()}")
            print(f"  Session: {os.path.basename(r['session_dir'])}")
            print(f"  Total attempts: {total_attempts}")
            print(f"  ✓ Succeeded: {r['total_records']} ({success_rate:.1f}%)")
            print(f"  ✗ Failed: {total_attempts - r['total_records']} ({100-success_rate:.1f}%)")
            print()
            print("  Failure breakdown:")
            print(f"    Incomplete outputs caught: {r['caught_incomplete']}")
            print(f"    Missing finish_reason: {r['caught_missing_finish']}")
            print(f"    Refusals: {r['caught_refusal']}")
            print()
            print("  Dataset quality:")
            print(f"    Total records: {r['total_records']}")
            print(
                f"    Complete: {r['complete']} "
                f"({r['complete']/r['total_records']*100 if r['total_records'] > 0 else 0:.1f}%)"
            )
            print(f"    Incomplete: {r['incomplete']}")
            print()

            if r["sample_incomplete"]:
                print("  Sample incomplete outputs saved:")
                for sample in r["sample_incomplete"]:
                    print(
                        f"    • {sample['length']:,} chars - ends: '...{sample['ending']}'"
                    )
                print()

            print("-" * 80)
            print()

        provider_groups = {}
        for r in analyses:
            provider_groups.setdefault(r["provider"], []).append(r)

        # Comparison summary (only if we have at least one session per provider)
        if "nahcrof" in provider_groups and "chutes" in provider_groups:
            nahcrof = provider_results["nahcrof"]
            chutes = provider_results["chutes"]

            nahcrof_total = (
                nahcrof["total_records"]
                + nahcrof["caught_incomplete"]
                + nahcrof["caught_refusal"]
                + nahcrof["caught_missing_finish"]
            )
            chutes_total = (
                chutes["total_records"]
                + chutes["caught_incomplete"]
                + chutes["caught_refusal"]
                + chutes["caught_missing_finish"]
            )

            nahcrof_success = (
                nahcrof["total_records"] / nahcrof_total * 100 if nahcrof_total > 0 else 0
            )
            chutes_success = (
                chutes["total_records"] / chutes_total * 100 if chutes_total > 0 else 0
            )

            print("=" * 80)
            print("COMPARISON SUMMARY")
            print("=" * 80)
            print()

            print(f"{'Metric':<30} {'nahcrof':>15} {'chutes':>15} {'Winner':>15}")
            print("-" * 80)
            print(
                f"{'Success Rate':<30} {nahcrof_success:>14.1f}% {chutes_success:>14.1f}% "
                f"{'chutes' if chutes_success > nahcrof_success else 'nahcrof':>15}"
            )
            print(
                f"{'Complete Stories':<30} {nahcrof['total_records']:>15} {chutes['total_records']:>15} "
                f"{'chutes' if chutes['total_records'] > nahcrof['total_records'] else 'nahcrof':>15}"
            )
            print(
                f"{'Incomplete Caught':<30} {nahcrof['caught_incomplete']:>15} {chutes['caught_incomplete']:>15} "
                f"{'chutes' if chutes['caught_incomplete'] < nahcrof['caught_incomplete'] else 'nahcrof':>15}"
            )
            print(
                f"{'Dataset Quality':<30} "
                f"{nahcrof['complete']}/{nahcrof['total_records']:>14} "
                f"{chutes['complete']}/{chutes['total_records']:>14} "
                f"{'BOTH' if nahcrof['incomplete'] == 0 and chutes['incomplete'] == 0 else 'varies':>15}"
            )
            print()

            print("=" * 80)
            print("VERDICT")
            print("=" * 80)
            print()

            if chutes_success >= 90 and nahcrof_success < 50:
                print("✅ CHUTES is significantly more reliable")
                print(
                    f"   • {chutes_success:.1f}% success vs {nahcrof_success:.1f}% for nahcrof"
                )
                print(
                    f"   • {chutes['caught_incomplete']} incomplete vs {nahcrof['caught_incomplete']} for nahcrof"
                )
                print()
                print("🎯 RECOMMENDATION: Use chutes (llm.chutes.ai) for production")

            elif nahcrof_success >= 90 and chutes_success < 50:
                print("✅ NAHCROF is significantly more reliable")
                print(
                    f"   • {nahcrof_success:.1f}% success vs {chutes_success:.1f}% for chutes"
                )
                print()
                print("🎯 RECOMMENDATION: Use nahcrof (ai.nahcrof.com) for production")

            elif nahcrof_success >= 90 and chutes_success >= 90:
                print("✅ BOTH providers are reliable")
                print(f"   • nahcrof: {nahcrof_success:.1f}% success")
                print(f"   • chutes: {chutes_success:.1f}% success")
                print()
                print("🎯 RECOMMENDATION: Use either provider based on cost/speed preference")

            else:
                print("⚠️  BOTH providers have reliability issues")
                print(f"   • nahcrof: {nahcrof_success:.1f}% success")
                print(f"   • chutes: {chutes_success:.1f}% success")
                print()
                print(
                    "🎯 RECOMMENDATION: Consider disabling streaming or testing other providers"
                )

    print()
    print("=" * 80)
    print("VALIDATION EFFECTIVENESS")
    print("=" * 80)
    print()
    print("The new validation is working correctly:")
    for provider, group in provider_groups.items():
        total_caught = sum(r["caught_incomplete"] + r["caught_refusal"] for r in group)
        total_records = sum(r["total_records"] for r in group)
        complete_records = sum(r["complete"] for r in group)
        print(f"  {provider}: Caught {total_caught} failures before they reached dataset")
        print(
            f"  → Dataset quality: {complete_records}/{total_records} complete "
            f"({complete_records/total_records*100 if total_records > 0 else 0:.1f}%)"
        )
    print()
    print("Without validation, incomplete outputs would pollute the training data!")

    # Save results (one entry per session so nothing gets overwritten)
    output = {"sessions": analyses}
    output_file = "tests/api-reliability-test/results.json"
    with open(output_file, "w") as f:
        json.dump(output, f, indent=2)
    print()
    print(f"Detailed results saved to: {output_file}")


if __name__ == '__main__':
    main()
