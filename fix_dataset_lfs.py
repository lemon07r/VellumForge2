#!/usr/bin/env python3
"""
Fix the Git LFS configuration that's causing newlines to display incorrectly.
The issue: dataset.jsonl is marked with -text flag in LFS, making it binary.
Solution: Delete and re-upload without LFS for JSONL files.
"""

from huggingface_hub import HfApi, hf_hub_download
from pathlib import Path
import os

def load_hf_token():
    env_file = Path('/home/lamim/Development/VellumForge2/.env')
    if env_file.exists():
        with open(env_file) as f:
            for line in f:
                if line.startswith('HUGGING_FACE_TOKEN='):
                    return line.split('=', 1)[1].strip()
    return None

def fix_gitattributes():
    """Remove dataset.jsonl from LFS configuration"""
    token = load_hf_token()
    if not token:
        print("Error: HF token not found")
        return False
    
    api = HfApi()
    repo_id = "lemon07r/VellumK2-Fantasy-DPO-Small-01"
    
    print("Fixing .gitattributes...")
    
    # Download current .gitattributes
    attr_path = hf_hub_download(
        repo_id=repo_id,
        filename=".gitattributes",
        repo_type="dataset",
        token=token
    )
    
    # Read and modify
    with open(attr_path, 'r') as f:
        lines = f.readlines()
    
    # Remove dataset.jsonl line
    new_lines = [line for line in lines if 'dataset.jsonl' not in line]
    
    # Write to temp file
    temp_attr = "/tmp/gitattributes_fixed"
    with open(temp_attr, 'w') as f:
        f.writelines(new_lines)
    
    print("Uploading fixed .gitattributes...")
    api.upload_file(
        path_or_fileobj=temp_attr,
        path_in_repo=".gitattributes",
        repo_id=repo_id,
        repo_type="dataset",
        token=token,
        commit_message="fix: Remove dataset.jsonl from LFS to fix newline rendering in viewer"
    )
    
    print("✓ .gitattributes fixed")
    return True

def reupload_dataset():
    """Re-upload dataset.jsonl without LFS"""
    token = load_hf_token()
    if not token:
        print("Error: HF token not found")
        return False
    
    api = HfApi()
    repo_id = "lemon07r/VellumK2-Fantasy-DPO-Small-01"
    
    dataset_file = Path("/home/lamim/Development/VellumForge2/output/session_2025-10-28T21-47-30/dataset.fixed.jsonl")
    
    if not dataset_file.exists():
        print(f"Error: Dataset file not found: {dataset_file}")
        return False
    
    print(f"Re-uploading dataset.jsonl without LFS...")
    print(f"  File size: {dataset_file.stat().st_size / (1024*1024):.2f} MB")
    
    # Delete the old LFS file first
    try:
        api.delete_file(
            path_in_repo="dataset.jsonl",
            repo_id=repo_id,
            repo_type="dataset",
            token=token,
            commit_message="chore: Remove LFS version of dataset.jsonl"
        )
        print("✓ Deleted old LFS file")
    except Exception as e:
        print(f"Note: Could not delete old file (may not exist): {e}")
    
    # Upload new version without LFS
    commit = api.upload_file(
        path_or_fileobj=str(dataset_file),
        path_in_repo="dataset.jsonl",
        repo_id=repo_id,
        repo_type="dataset",
        token=token,
        commit_message="fix: Re-upload dataset.jsonl as regular text file for proper newline rendering"
    )
    
    print(f"✓ Dataset re-uploaded")
    print(f"  Commit: {commit.commit_url}")
    return True

if __name__ == "__main__":
    print("="*70)
    print("  Fixing dataset.jsonl LFS configuration")
    print("="*70)
    print()
    print("Issue: dataset.jsonl is stored in Git LFS with -text flag,")
    print("       causing HuggingFace viewer to treat it as binary")
    print("       and display literal \\n\\n instead of line breaks.")
    print()
    print("Solution: Remove LFS configuration and re-upload as text file.")
    print()
    
    if fix_gitattributes():
        print()
        if reupload_dataset():
            print()
            print("="*70)
            print("  ✓ FIX COMPLETE")
            print("="*70)
            print()
            print("The dataset viewer should now properly render newlines.")
            print("URL: https://huggingface.co/datasets/lemon07r/VellumK2-Fantasy-DPO-Small-01")
        else:
            print("\n✗ Failed to re-upload dataset")
    else:
        print("\n✗ Failed to fix .gitattributes")
