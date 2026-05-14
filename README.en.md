# TATAI - Terminal Access Control System

TATAI is a lightweight application process management platform providing a sleek Web interface to unify the management of various service processes. Whether you are running Docker containers, Java JAR packages, Nginx services, or custom scripts, TATAI allows you to perform operations such as start/stop, monitoring, and log viewing through a single console.

## Core Features

### 📦 Multi-type Application Management

* **Docker Containers**: Manage running containers with automatic identification of mapped ports
* **Java JAR Applications**: Support for multiple JDK version switching and configurable JVM memory parameters
* **Nginx Services**: Parse configuration files to obtain listening ports; support for reloading configurations
* **Other Custom Processes**: Any command-line program, supporting start/stop detection commands

### 🛡️ Intelligent Process Guardian

* Automatically detect application crashes and restart
* Configurable maximum retry attempts and statistical time windows
* Exponential backoff strategy to prevent resource exhaustion from frequent restarts
* Restart history logs for easier troubleshooting

### 📡 Real-time Log Stream

* WebSocket persistent connection pushing with millisecond latency
* Support for historical log retrieval (latest 50 lines)
* Independent log file storage per application
* Free toggle between auto-scroll and manual scroll

### 📊 System Resource Monitoring

* Real-time display of CPU / Memory / Disk usage
* TOP 5 process ranking (CPU / Memory consumption)
* Disk cleaning suggestions: Smart identification of large files with one-click clearing support

### 🔔 Webhook Alerts

* Automatic alert notifications sent upon application crash
* Support for custom Webhook URLs
* Configurable trigger event types
* Built-in test function to verify connectivity

### 👥 User Permission Management

* Multi-user support with role separation (admin / operator)
* Admins can create/disable users
* Admins can reset passwords for regular users
* Regular users can modify their own passwords

### 🎨 Modern Interface

* Dark theme design, eye-friendly and professional
* Responsive layout supporting various screen sizes
* Application list supports filtering by type/status
* Drag-and-drop application sorting (Under Development)

## Quick Start

### Deployment

1. **Download the latest version**

```bash
git clone https://github.com/your-repo/tatai.git
cd tatai

```

2. **Start the service**

```bash
# Run directly
./tatai

# Or use go run
go run main.go

```

3. **Access the Console**
   Open your browser and visit `http://localhost:3000`

### Default Credentials

| Username | Password | Role |
| --- | --- | --- |
| admin | 123456 | Administrator |

> Please change your password immediately after the first login.

## Application Configuration Examples

### Java JAR Application

```json
{
  "type": "jar",
  "appName": "user-service",
  "jdkKey": "jdk17",
  "jarPath": "/opt/apps/user-service.jar",
  "memory": "512m",
  "ports": "[8080, 8081]"
}

```

### Custom Process

```json
{
  "type": "other",
  "startCmd": "./server --port 3000",
  "stopCmd": "pkill -f server",
  "checkCmd": "pgrep -f server",
  "ports": "[3000]"
}

```

### Docker Container

```json
{
  "type": "docker",
  "appName": "nginx-proxy",
  "cmd": "docker run -d -p 80:80 nginx"
}

```

## UI Preview

### Dashboard

* Three ring progress bars showing CPU / Memory / Disk usage
* Quick access: Process ranking, Disk cleaning
* Top 3 core service cards with one-click start/stop
* List of recently active applications

### App Center

* Statistics cards: Total / Running / Guarded / Abnormal
* App list: Name, Type, Status, Ports, Remarks
* Drawer-style editing panel: Configuration modification + Real-time logs
* Port details popup: Comparison of Expected Ports vs. Actual Listening Ports

### Settings

* User Management: Create / Enable / Disable / Reset Password
* Webhook Config: Notification URL settings and testing

## Guardianship Strategy Description

When application guarding is enabled, TATAI continuously monitors the application's running status:

| Parameter | Description | Default Value |
| --- | --- | --- |
| Max Retries | Maximum allowed restarts within the window period | 5 times |
| Stats Window | Time window to reset the counter | 60 seconds |
| Backoff Base | Incremental base for restart intervals | 2 seconds |
| Max Backoff | Maximum wait time for a single retry | 60 seconds |

Restart delay calculation formula: $delay = \min(Base \times 2^{(count-1)}, MaxBackoff)$

Example:

* 1st crash: wait 2 seconds
* 2nd crash: wait 4 seconds
* 3rd crash: wait 8 seconds
* 4th crash: wait 16 seconds
* 5th crash: wait 32 seconds

## Port Match Detection

For JAR and Other type applications, TATAI will:

1. Read the expected ports configured by the user (JSON array format)
2. Obtain the actual listening ports of the process via `lsof -p {PID}`
3. Display the match status on the interface for comparison:
* ✅ **matched**: Expected ports match actual ports
* ⚠️ **mismatch**: Ports do not match; click to view details
* ❓ **unknown**: Unable to retrieve actual ports



## Storage and Data

* **Database**: SQLite, located at `./data/tatai.db`
* **Log Files**: `./logs/{app_name}_{app_id}.log`
* **Config File**: `./config.yaml` (Optional, supports environment variable overrides)

## Technical Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Browser (Vue 3)                   │
│              Naive UI / WebSocket Client             │
└─────────────────────────┬───────────────────────────┘
                          │ HTTP / WS
┌─────────────────────────▼───────────────────────────┐
│                   Go Backend (Chi)                   │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌───────────┐  │
│  │   Auth  │ │Manager  │ │ Guard   │ │  Monitor  │  │
│  │  (JWT)  │ │(Process)│ │(Daemon) │ │ (System)  │  │
│  └─────────┘ └─────────┘ └─────────┘ └───────────┘  │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌───────────┐  │
│  │ Notify  │ │Database │ │ Logs    │ │  JDKPool  │  │
│  │(Webhook)│ │(SQLite) │ │(Stream) │ │           │  │
│  └─────────┘ └─────────┘ └─────────┘ └───────────┘  │
└─────────────────────────────────────────────────────┘

```

## License

MIT License

---

**TATAI** - Take control of your services. 🚀