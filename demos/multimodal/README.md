# Real-time Webcam Vision Model Demo

This demo allows you to interact with a vision model in real-time using your webcam. The model can analyze the video feed and answer questions about what it sees.

## Credits

This demo is based on the excellent work by [ngxson/smolvlm-realtime-webcam](https://github.com/ngxson/smolvlm-realtime-webcam). Thank you for creating this impressive demonstration!

## Prerequisites

Before running this demo, you need:

1. **Docker Model Runner** - Either through Docker Desktop or standalone installation
2. **The SmolVLM model** - Specifically `ai/smolvlm:500M-Q8_0`

## Setup Instructions

You have two options for setting up Docker Model Runner:

### Option A: Using Docker Desktop (Easiest)

This is the recommended approach for most users.

1. **Enable Docker Model Runner**
   - Open Docker Desktop settings
   - Go to the **AI** tab
   - Select **Enable Docker Model Runner**

2. **Enable TCP Support and CORS**
   - In the same settings page, select **Enable host-side TCP support**
   - Set the **Port** to `12434` (default)
   - In **CORS Allows Origins**, add `*` or the specific origin where you'll open the HTML file
   
   For detailed instructions, see the [Docker Model Runner documentation](https://docs.docker.com/ai/model-runner/get-started/#enable-docker-model-runner).

3. **Pull the Model**
   - Open Docker Desktop
   - Go to the **Models** tab → **Docker Hub**
   - Search for `ai/smolvlm:500M-Q8_0` and click **Pull**
   
   Or use the CLI:
   ```bash
   docker model pull ai/smolvlm:500M-Q8_0
   ```

### Option B: Using Standalone Docker Model Runner

If you prefer not to use Docker Desktop, you can run Docker Model Runner directly:

1. **Install Docker Model Runner**
   
   Follow the installation instructions in the [main README](../../README.md) for your platform.

2. **Pull the Model**
   ```bash
   docker model pull ai/smolvlm:500M-Q8_0
   ```

> **Note:** TCP support is enabled by default on port `12434` when using Docker Engine.

## Running the Demo

1. **Open the Demo**
   - Simply open `demo.html` in your web browser
   - You can open it directly from your file system or serve it with a local web server

2. **Grant Camera Permission**
   - Your browser will ask for camera access
   - Click "Allow" to grant permission

3. **Configure the Demo**
   - **Base API**: By default set to `http://127.0.0.1:12434/engines/llama.cpp`
     - Change the port if you configured Docker Model Runner on a different port
   - **Instruction**: Enter what you want the model to analyze (default: "What do you see?")
     - Examples: "Describe the scene", "What objects can you see?", "What is the person doing?"
   - **Interval**: Choose how often to send requests to the model (default: 500ms)
     - Shorter intervals = more responsive but higher resource usage
     - Longer intervals = lower resource usage but less real-time feel

4. **Start the Interaction**
   - Click the **Start** button
   - The model will begin analyzing your webcam feed
   - Responses will appear in the **Response** text area
   - Click **Stop** when you're done

## Learn More

- [Community Slack Channel](https://app.slack.com/client/T0JK1PCN6/C09H9P5E57B)
- [Docker Model Runner Documentation](https://docs.docker.com/ai/model-runner/)
- [Original Demo by ngxson](https://github.com/ngxson/smolvlm-realtime-webcam)
- [SmolVLM Model Information](https://huggingface.co/HuggingFaceTB/SmolVLM-Instruct)
