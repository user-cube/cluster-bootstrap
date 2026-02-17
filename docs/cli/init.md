# init

```bash
cluster-bootstrap-cli init
```

Interactive setup to configure encryption and create per-environment secrets files.

## What it does

1. Prompts for encryption provider (age, AWS KMS, GCP KMS, or git-crypt)
2. For SOPS providers: collects the encryption key, generates `.sops.yaml`, creates encrypted `secrets.<env>.enc.yaml` files
3. For git-crypt: verifies `git-crypt init` has been run, ensures `.gitattributes` has the git-crypt pattern, creates plaintext `secrets.<env>.yaml` files (encrypted transparently on commit)

## Flags

| Flag | Description |
|------|-------------|
| `--provider` | Encryption provider: `age`, `aws-kms`, `gcp-kms`, or `git-crypt` |
| `--age-key-file` | Path to age public key file |
| `--kms-arn` | AWS KMS key ARN |
| `--gcp-kms-key` | GCP KMS key resource ID |
| `--output-dir` | Output directory (default: current directory, or `--base-dir` if set) |
