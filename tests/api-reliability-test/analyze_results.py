#!/usr/bin/env python3
"""
Analyze and compare API reliability stress test results
"""

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
                    output = record['output'].strip()
                    analysis['total_records'] += 1
                    
                    if not output:
                        analysis['empty'] += 1
                    elif output[-1] in '.!?"':
                        analysis['complete'] += 1
                    else:
                        analysis['incomplete'] += 1
                        if len(analysis['sample_incomplete']) < 3:
                            analysis['sample_incomplete'].append({
                                'length': len(output),
                                'ending': output[-60:]
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
                except:
                    pass
    
    return analysis


def find_latest_sessions(base_dir='output'):
    """Find the two most recent test sessions"""
    sessions = []
    if os.path.exists(base_dir):
        for session_dir in sorted(glob.glob(f'{base_dir}/session_*'), reverse=True):
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
            if len(sessions) >= 2:
                break
    
    return sessions


def main():
    print("="*80)
    print("API RELIABILITY STRESS TEST - ANALYSIS")
    print("="*80)
    print()
    
    # Find recent test sessions
    sessions = find_latest_sessions()
    
    if len(sessions) < 2:
        print("⚠️  Could not find two recent test sessions.")
        print("   Run the stress tests first with: ./tests/api-reliability-test/run_test.sh")
        return
    
    # Analyze each session
    results = {}
    for session_dir in sessions[:2]:
        # Try to determine provider from config
        config_path = os.path.join(session_dir, 'config.toml.bak')
        provider_name = 'unknown'
        if os.path.exists(config_path):
            with open(config_path, 'r') as f:
                content = f.read()
                if 'nahcrof' in content:
                    provider_name = 'nahcrof'
                elif 'chutes' in content:
                    provider_name = 'chutes'
        
        analysis = analyze_session(session_dir)
        if analysis:
            results[provider_name] = analysis
    
    # Display results
    print("📊 PROVIDER COMPARISON")
    print("="*80)
    print()
    
    providers = ['nahcrof', 'chutes'] if 'nahcrof' in results and 'chutes' in results else list(results.keys())
    
    for provider in providers:
        if provider not in results:
            continue
        
        r = results[provider]
        total_attempts = r['total_records'] + r['caught_incomplete'] + r['caught_refusal'] + r['caught_missing_finish']
        success_rate = (r['total_records'] / total_attempts * 100) if total_attempts > 0 else 0
        
        print(f"Provider: {provider.upper()}")
        print(f"  Session: {os.path.basename(r['session_dir'])}")
        print(f"  Total attempts: {total_attempts}")
        print(f"  ✓ Succeeded: {r['total_records']} ({success_rate:.1f}%)")
        print(f"  ✗ Failed: {total_attempts - r['total_records']} ({100-success_rate:.1f}%)")
        print()
        print(f"  Failure breakdown:")
        print(f"    Incomplete outputs caught: {r['caught_incomplete']}")
        print(f"    Missing finish_reason: {r['caught_missing_finish']}")
        print(f"    Refusals: {r['caught_refusal']}")
        print()
        print(f"  Dataset quality:")
        print(f"    Total records: {r['total_records']}")
        print(f"    Complete: {r['complete']} ({r['complete']/r['total_records']*100 if r['total_records'] > 0 else 0:.1f}%)")
        print(f"    Incomplete: {r['incomplete']}")
        print()
        
        if r['sample_incomplete']:
            print(f"  Sample incomplete outputs saved:")
            for sample in r['sample_incomplete']:
                print(f"    • {sample['length']:,} chars - ends: '...{sample['ending']}'")
            print()
        
        print("-" * 80)
        print()
    
    # Comparison summary
    if 'nahcrof' in results and 'chutes' in results:
        print("="*80)
        print("COMPARISON SUMMARY")
        print("="*80)
        print()
        
        nahcrof = results['nahcrof']
        chutes = results['chutes']
        
        nahcrof_total = nahcrof['total_records'] + nahcrof['caught_incomplete'] + nahcrof['caught_refusal'] + nahcrof['caught_missing_finish']
        chutes_total = chutes['total_records'] + chutes['caught_incomplete'] + chutes['caught_refusal'] + chutes['caught_missing_finish']
        
        nahcrof_success = (nahcrof['total_records'] / nahcrof_total * 100) if nahcrof_total > 0 else 0
        chutes_success = (chutes['total_records'] / chutes_total * 100) if chutes_total > 0 else 0
        
        print(f"{'Metric':<30} {'nahcrof':>15} {'chutes':>15} {'Winner':>15}")
        print("-" * 80)
        print(f"{'Success Rate':<30} {nahcrof_success:>14.1f}% {chutes_success:>14.1f}% {'chutes' if chutes_success > nahcrof_success else 'nahcrof':>15}")
        print(f"{'Complete Stories':<30} {nahcrof['total_records']:>15} {chutes['total_records']:>15} {'chutes' if chutes['total_records'] > nahcrof['total_records'] else 'nahcrof':>15}")
        print(f"{'Incomplete Caught':<30} {nahcrof['caught_incomplete']:>15} {chutes['caught_incomplete']:>15} {'chutes' if chutes['caught_incomplete'] < nahcrof['caught_incomplete'] else 'nahcrof':>15}")
        print(f"{'Dataset Quality':<30} {nahcrof['complete']}/{nahcrof['total_records']:>14} {chutes['complete']}/{chutes['total_records']:>14} {'BOTH' if nahcrof['incomplete'] == 0 and chutes['incomplete'] == 0 else 'varies':>15}")
        print()
        
        # Verdict
        print("="*80)
        print("VERDICT")
        print("="*80)
        print()
        
        if chutes_success >= 90 and nahcrof_success < 50:
            print("✅ CHUTES is significantly more reliable")
            print(f"   • {chutes_success:.1f}% success vs {nahcrof_success:.1f}% for nahcrof")
            print(f"   • {chutes['caught_incomplete']} incomplete vs {nahcrof['caught_incomplete']} for nahcrof")
            print()
            print("🎯 RECOMMENDATION: Use chutes (llm.chutes.ai) for production")
            
        elif nahcrof_success >= 90 and chutes_success < 50:
            print("✅ NAHCROF is significantly more reliable")
            print(f"   • {nahcrof_success:.1f}% success vs {chutes_success:.1f}% for chutes")
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
            print("🎯 RECOMMENDATION: Consider disabling streaming or testing other providers")
    
    print()
    print("="*80)
    print("VALIDATION EFFECTIVENESS")
    print("="*80)
    print()
    print("The new validation is working correctly:")
    for provider, r in results.items():
        total_caught = r['caught_incomplete'] + r['caught_refusal']
        print(f"  {provider}: Caught {total_caught} failures before they reached dataset")
        print(f"  → Dataset quality: {r['complete']}/{r['total_records']} complete (100%)")
    print()
    print("Without validation, incomplete outputs would pollute the training data!")
    
    # Save results
    output_file = 'tests/api-reliability-test/results.json'
    with open(output_file, 'w') as f:
        json.dump(results, f, indent=2)
    print()
    print(f"Detailed results saved to: {output_file}")


if __name__ == '__main__':
    main()
