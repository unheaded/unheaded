# TODO Security Summary for Unheaded Project

**Overall Assessment:**

The Unheaded project is a complex, cloud-native application with a microservices architecture. While the project is functionally sophisticated, a security assessment has revealed several critical and high-severity vulnerabilities that require immediate attention. The most pervasive issue is a complete lack of authentication and authorization across all services, leaving the system exposed to unauthorized access, data exfiltration, and modification.

**Critical Vulnerabilities:**

*   **No Authentication or Authorization on All Services:** None of the services (`busboy`, `timeguru`, `captain`) have any form of authentication or authorization. This is the most critical issue and needs to be addressed immediately.
    *   **Affected Services:** `busboy`, `timeguru`, `captain`
    *   **Recommendation:** Implement a unified authentication and authorization mechanism for all services. This could be based on API keys, JWT tokens, or a more robust solution like OAuth2.

*   **`busboy` Admin Endpoints Effectively Disabled:** The `busboy` service has admin endpoints that are intended to be protected by an API key. However, there is no mechanism to set the API key, which means the admin endpoints are disabled. This is a critical issue because it prevents administrators from managing the service.
    *   **Affected Service:** `busboy`
    *   **Recommendation:** Add a mechanism to set the admin API key, for example, via a command-line flag or an environment variable.

**High-Severity Vulnerabilities:**

*   **Permissive CORS Policies:** The `busboy` and `timeguru` services have permissive CORS policies that allow all origins. This could allow malicious websites to make requests to the services on behalf of users.
    *   **Affected Services:** `busboy`, `timeguru`
    *   **Recommendation:** Restrict the CORS policy to a specific set of trusted domains.

*   **Lack of TLS:** None of the services use TLS to encrypt communication. This means that all traffic to and from the services is sent in cleartext, which could allow an attacker to intercept and read sensitive data.
    *   **Affected Services:** `busboy`, `timeguru`, `captain`
    *   **Recommendation:** Enable TLS on all services. Use a service like Let's Encrypt to obtain free TLS certificates.

*   **Privileged `cuirass` Container:** The `cuirass` service in the `docker-compose.yml` file runs with powerful capabilities (`CAP_SYS_ADMIN`, `CAP_NET_ADMIN`) and mounts the Docker socket. This gives it control over the Docker daemon and is a major security risk.
    *   **Affected Service:** `cuirass`
    *   **Recommendation:** Apply the principle of least privilege. Remove unnecessary capabilities and avoid mounting the Docker socket if possible. If access to the Docker API is required, consider using a more secure alternative, such as a proxy that exposes a limited subset of the API.

**Medium-Severity Vulnerabilities:**

*   **Hardcoded Passwords:** The `setup-host.sh` script contains a hardcoded LXD trust password, and the `docker-compose.yml` file contains a hardcoded Grafana password.
    *   **Affected Files:** `scripts/setup-host.sh`, `docker-compose.yml`
    *   **Recommendation:** Remove hardcoded passwords and use a secret management solution like HashiCorp Vault or AWS Secrets Manager.

*   **Services Running as Root:** The services in the Docker containers run as the `root` user by default.
    *   **Affected Services:** `busboy`, `timeguru`, `captain`
    *   **Recommendation:** Configure the services to run as a non-root user.

*   **Lack of Input Validation:** The `timeguru` and `captain` services have endpoints that take an ID from the URL path without proper validation. This could lead to path traversal vulnerabilities.
    *   **Affected Services:** `timeguru`, `captain`
    *   **Recommendation:** Implement input validation to ensure that the IDs are in the correct format.

**Action Plan:**

1.  **Implement Authentication and Authorization:** This is the highest priority. All other vulnerabilities are secondary to this.
2.  **Fix `busboy` Admin Endpoints:** Add a mechanism to set the admin API key.
3.  **Restrict CORS Policies:** Lock down the CORS policies to a specific set of trusted domains.
4.  **Enable TLS:** Enable TLS on all services.
5.  **Harden `cuirass` Container:** Apply the principle of least privilege to the `cuirass` container.
6.  **Remove Hardcoded Passwords:** Use a secret management solution.
7.  **Run Services as Non-Root:** Configure the services to run as a non-root user.
8.  **Implement Input Validation:** Add input validation to all endpoints that take user-supplied input.
