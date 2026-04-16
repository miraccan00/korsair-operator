FROM mcr.microsoft.com/devcontainers/go:1.25-bookworm

# ── Fix: Yarn apt repo GPG key expired — Yarn is not needed ──────────────────
RUN rm -f /etc/apt/sources.list.d/yarn.list

# ── Docker CE CLI ─────────────────────────────────────────────────────────────
# Install the official Docker CLI so we can talk to the host Docker daemon
# (Docker Desktop / Docker Engine). The daemon itself runs on the host, not here.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl gnupg \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg \
       | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && chmod a+r /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
        https://download.docker.com/linux/debian bookworm stable" \
       > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends docker-ce-cli docker-buildx-plugin \
    && rm -rf /var/lib/apt/lists/*

# ── kubectl ───────────────────────────────────────────────────────────────────
# Pinned to match the Kubernetes version used in kind (v1.32.x).
# Multi-arch: detects amd64 or arm64 at build time.
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL "https://dl.k8s.io/release/v1.32.0/bin/linux/${ARCH}/kubectl" \
       -o /usr/local/bin/kubectl \
    && chmod +x /usr/local/bin/kubectl

# ── kind ──────────────────────────────────────────────────────────────────────
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL "https://kind.sigs.k8s.io/dl/v0.25.0/kind-linux-${ARCH}" \
       -o /usr/local/bin/kind \
    && chmod +x /usr/local/bin/kind

# ── Helm ──────────────────────────────────────────────────────────────────────
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# ── Tilt ──────────────────────────────────────────────────────────────────────
# Tilt release archives use x86_64 (not amd64) and arm64 — map dpkg arch names.
RUN ARCH=$(dpkg --print-architecture) \
    && TILT_ARCH=$([ "$ARCH" = "amd64" ] && echo "x86_64" || echo "arm64") \
    && curl -fsSL "https://github.com/tilt-dev/tilt/releases/download/v0.33.21/tilt.0.33.21.linux.${TILT_ARCH}.tar.gz" \
       | tar -xz -C /usr/local/bin tilt \
    && chmod +x /usr/local/bin/tilt

# ── kubebuilder ───────────────────────────────────────────────────────────────
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v4.12.0/kubebuilder_linux_${ARCH}" \
       -o /usr/local/bin/kubebuilder \
    && chmod +x /usr/local/bin/kubebuilder

# ── kubectx + kubens ──────────────────────────────────────────────────────────
RUN git clone --depth=1 https://github.com/ahmetb/kubectx /opt/kubectx \
    && ln -s /opt/kubectx/kubectx /usr/local/bin/kubectx \
    && ln -s /opt/kubectx/kubens  /usr/local/bin/kubens

# ── vscode user: .kube dir + shell completions + aliases ─────────────────────
RUN mkdir -p /home/vscode/.kube \
    && chown vscode:vscode /home/vscode/.kube \
    && echo 'source <(kubectl completion bash)'             >> /home/vscode/.bashrc \
    && echo 'source <(kind completion bash)'                >> /home/vscode/.bashrc \
    && echo 'source <(tilt completion bash)'                >> /home/vscode/.bashrc \
    && echo 'source <(helm completion bash)'                >> /home/vscode/.bashrc \
    && echo 'source /opt/kubectx/completion/kubectx.bash'  >> /home/vscode/.bashrc \
    && echo 'source /opt/kubectx/completion/kubens.bash'   >> /home/vscode/.bashrc \
    && echo 'alias k=kubectl'                               >> /home/vscode/.bashrc \
    && echo 'complete -F __start_kubectl k'                 >> /home/vscode/.bashrc
