FROM mcr.microsoft.com/devcontainers/go:1-1.25-bookworm

RUN apt-get update && apt-get install -y \
    make \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js and npm for claude-code
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
    && apt-get install -y nodejs

RUN cd /tmp && \
    go mod init temp && \
    go install github.com/air-verse/air@v1.61.5 && \
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
    go install github.com/go-delve/delve/cmd/dlv@latest && \
    rm -rf /tmp/go.mod /tmp/go.sum

RUN usermod -aG sudo vscode && \
    echo "vscode ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Switch to vscode user before installing Claude Code
USER vscode
WORKDIR /home/vscode

RUN mkdir -p ~/.npm-global && \
    npm config set prefix '~/.npm-global'

# Install Claude Code using npm
RUN npm install -g @anthropic-ai/claude-code

# Add npm global bin to PATH
RUN echo 'export PATH="$HOME/.npm-global/bin:$PATH"' >> ~/.bashrc && \
    echo 'export PATH="$HOME/.npm-global/bin:$PATH"' >> ~/.profile

WORKDIR /workspace
