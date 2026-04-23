export interface RunnerConfig {
  slug: string;
  name: string;
  tags?: string[];
  concurrencyLimit?: number;
  gpuCapable?: boolean;
}

export interface CommandRequest {
  targetType: 'slug' | 'tag';
  targetValue: string;
  payload: Record<string, any>;
  maxRetries?: number;
  timeoutSecs?: number;
  deadline?: string;
}

export interface RunnerFilter {
  status?: 'offline' | 'online' | 'orphaned';
  tags?: string[];
}

export interface MetricOpts {
  resolution?: 'raw' | '1m' | '5m';
  page?: number;
  pageSize?: number;
}

export interface CommandEvent {
  type: string;
  data: any;
}

export class RunnerClient {
  private baseURL: string;
  private token?: string;
  private fetchOptions: RequestInit;

  constructor(baseURL: string, options?: { token?: string; fetchOptions?: RequestInit }) {
    this.baseURL = baseURL.replace(/\/$/, '');
    this.token = options?.token;
    this.fetchOptions = options?.fetchOptions || {};
  }

  async registerRunner(config: RunnerConfig): Promise<any> {
    return this.request('POST', '/api/v1/runners/register', {
      slug: config.slug,
      name: config.name,
      tags: config.tags || [],
      concurrency_limit: config.concurrencyLimit || 4,
      gpu_capable: config.gpuCapable || false,
    });
  }

  async deregisterRunner(slug: string): Promise<void> {
    await this.request('DELETE', `/api/v1/runners/${slug}`);
  }

  async listRunners(filter?: RunnerFilter): Promise<any[]> {
    const params = new URLSearchParams();
    if (filter?.status) params.append('status', filter.status);
    if (filter?.tags) filter.tags.forEach(tag => params.append('tags', tag));
    
    const query = params.toString();
    const url = query ? `/api/v1/runners?${query}` : '/api/v1/runners';
    
    return this.request('GET', url);
  }

  async getRunner(slug: string): Promise<any> {
    return this.request('GET', `/api/v1/runners/${slug}`);
  }

  async sendCommand(req: CommandRequest): Promise<any> {
    return this.request('POST', '/api/v1/commands', {
      target_type: req.targetType,
      target_value: req.targetValue,
      payload: req.payload,
      max_retries: req.maxRetries || 0,
      timeout_secs: req.timeoutSecs || 300,
      deadline: req.deadline,
    });
  }

  async getCommand(id: string): Promise<any> {
    return this.request('GET', `/api/v1/commands/${id}`);
  }

  async killCommand(id: string): Promise<void> {
    await this.request('POST', `/api/v1/commands/${id}/kill`);
  }

  async getLogs(commandId: string, page = 1, pageSize = 100): Promise<any[]> {
    return this.request('GET', `/api/v1/commands/${commandId}/logs?page=${page}&page_size=${pageSize}`);
  }

  async getMetrics(commandId: string, opts?: MetricOpts): Promise<any[]> {
    const resolution = opts?.resolution || 'raw';
    const page = opts?.page || 1;
    const pageSize = opts?.pageSize || 100;
    
    return this.request('GET', 
      `/api/v1/commands/${commandId}/metrics?resolution=${resolution}&page=${page}&page_size=${pageSize}`
    );
  }

  watchCommands(slug: string): EventSource {
    const url = `${this.baseURL}/api/v1/runners/${slug}/sse`;
    const eventSource = new EventSource(url);
    
    eventSource.addEventListener('error', (event) => {
      console.error('SSE connection error:', event);
    });

    return eventSource;
  }

  watchLogs(commandId: string, onLog: (log: any) => void): () => void {
    let stopped = false;
    let lastSeq = 0;

    const poll = async () => {
      while (!stopped) {
        try {
          const logs = await this.getLogs(commandId, 1, 100);
          
          for (const log of logs) {
            if (log.seq > lastSeq) {
              onLog(log);
              lastSeq = log.seq;
            }
          }
        } catch (err) {
          console.error('Error polling logs:', err);
        }

        await new Promise(resolve => setTimeout(resolve, 1000));
      }
    };

    poll();

    return () => {
      stopped = true;
    };
  }

  private async request(method: string, path: string, body?: any): Promise<any> {
    const url = `${this.baseURL}${path}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    const options: RequestInit = {
      ...this.fetchOptions,
      method,
      headers: { ...headers, ...this.fetchOptions.headers },
    };

    if (body) {
      options.body = JSON.stringify(body);
    }

    const response = await fetch(url, options);

    if (response.status === 204) {
      return;
    }

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Unknown error' }));
      throw new Error(error.error || `Request failed: ${response.status}`);
    }

    return response.json();
  }
}

export default RunnerClient;
