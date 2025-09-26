# VMware API Runner

A Go-based tool for executing scripts inside VMware virtual machines through the vSphere API. Automates guest OS operations without direct SSH access.

## What Does This Tool Do?

This tool connects to your VMware vCenter/ESXi, finds a specific virtual machine, uploads a script to it, executes the script, and downloads the output back to your local machine. Perfect for:

- **Automated maintenance** tasks on VMs
- **Health checks** and monitoring
- **Batch operations** across multiple VMs
- **CI/CD pipelines** involving VM configuration

## Prerequisites

### 1. VMware Environment

- vCenter Server or ESXi host (6.5+ recommended)
- User account with permissions to:
  - Access the VM
  - Use Guest Operations API
  - Perform file operations in the guest OS

### 2. Guest VM Requirements

- VMware Tools installed and running
- Guest OS credentials (username/password)
- sudo privileges for the script execution
- bash shell available

### 3. Local Environment

- Go 1.16 or higher
- Network access to vCenter/ESXi host (port 443)

## Quick Start Guide

### Project Structure

```text
vmware-runner/
├── main.go              # Main application code
├── script.sh            # Script to execute in VM
├── .env                 # Environment variables (create)
├── go.mod               # Go dependencies
└── README.md            # This file
```

### Step 1: Install Dependencies

```bash
# Initialize Go module (if needed)
go mod init vmware-runner
go mod tidy
```

### Step 2: Configure Environment Variables

Create a `.env` file in the project root:

```bash
# vCenter connection
VCENTER_HOST=your-vcenter.example.com
VCENTER_USER=administrator@vsphere.local
VCENTER_PASS=your-password
VCENTER_INSECURE=true  # set to false for production
VCENTER_DATACENTER=Datacenter  # optional

# Target VM settings
VM_NAME=ubuntu-server-01
GUEST_USER=ubuntu
GUEST_PASS=guest-password
```

**Security Note:** For production, use secure credential storage instead of `.env` files.

### Step 3: Create Your Script

Create a `script.sh` file that will run inside the VM:

```bash
#!/bin/bash
# Example diagnostic script
echo "=== System Information ==="
uname -a
echo "=== Disk Usage ==="
df -h
echo "=== Memory Usage ==="
free -h
```

### Step 4: Run the Tool

```bash
go run main.go
```

If successful, you'll see output like:

```text
✅ successful authentication: ubuntu@ubuntu-server-01
✅ Script uploaded to guest: /tmp/govmomi_script_1234567890.sh (size=150)
▶ Script running! (pid=1234). Waiting for completion...
✔ Script completed (exitCode=0)
✅ Output saved to file: ubuntu-server-01.txt
```

## How It Works

1. **Authentication**
   - Connects to vCenter using provided credentials
   - Validates guest OS credentials using VMware Tools

2. **File Transfer**
   - Creates unique temporary files in `/tmp/` on the guest VM
   - Uploads your `script.sh` using vSphere file transfer API

3. **Script Execution**
   - Executes the script with bash and sudo privileges
   - Redirects output to a temporary file
   - Monitors process completion with timeout (5 minutes)

4. **Result Collection**
   - Downloads the output file from guest VM
   - Saves results to `[VM_NAME].txt` locally
   - Cleans up temporary files from guest

## Configuration Options

### Environment Variables

| Variable             | Description                           | Required |
|---------------------|---------------------------------------|----------|
| VCENTER_HOST         | vCenter/ESXi hostname or IP          | Yes      |
| VCENTER_USER         | vSphere username                      | Yes      |
| VCENTER_PASS         | vSphere password                      | Yes      |
| VCENTER_INSECURE     | Skip SSL verification                 | No (default: false) |
| VCENTER_DATACENTER   | Specific datacenter                   | No       |
| VM_NAME              | Target virtual machine name           | Yes      |
| GUEST_USER           | Guest OS username                     | Yes      |
| GUEST_PASS           | Guest OS password                     | Yes      |

### Script Requirements

Your `script.sh` must:

- Have valid bash syntax
- Include shebang (`#!/bin/bash`)
- Be self-contained (no interactive prompts)
- Handle its own error logging

## Example Use Cases

### 1. System Health Check

```bash
#!/bin/bash
echo "=== $(date) ==="
echo "Uptime: $(uptime)"
echo "CPU Load: $(cat /proc/loadavg)"
echo "Memory: $(free -h)"
```

### 2. Service Monitoring

```bash
#!/bin/bash
for service in nginx mysql docker; do
    systemctl is-active $service && echo "✓ $service: RUNNING" || echo "✗ $service: STOPPED"
done
```

### 3. Application Deployment

```bash
#!/bin/bash
cd /opt/myapp
git pull origin main
docker-compose down
docker-compose up -d
```

## Troubleshooting

### Common Issues

**Connection Failed:**
- Verify vCenter hostname and credentials
- Check network connectivity to vCenter
- Ensure SSL certificates are trusted (or use `VCENTER_INSECURE=true`)

**Authentication Failed:**
- Confirm guest OS credentials are correct
- Verify VMware Tools is running in the VM
- Check guest account has necessary permissions

**Script Not Executing:**
- Ensure script has proper bash syntax
- Verify sudo privileges for the guest user
- Check script file permissions on upload

### Debug Mode

Add debug logging by modifying the Go code:

```go
// Enable verbose logging
client, err := govmomi.NewClient(ctx, u, insecure)
if err != nil {
    log.Printf("Detailed error: %+v", err)
}
```

### Security Considerations

- ✅ Use service accounts with minimal required permissions
- ✅ Store credentials in secure vaults (not in files)
- ✅ Set `VCENTER_INSECURE=false` in production
- ✅ Regularly rotate passwords and tokens
- ✅ Audit script contents before execution

