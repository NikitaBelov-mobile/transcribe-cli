using System;
using System.Net.Http;
using System.Net.Http.Json;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace TranscribeDesktop.WinUI.Services;

public sealed class TranscribeApiClient : IDisposable
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    private readonly HttpClient _http;

    public TranscribeApiClient(string baseUrl)
    {
        _http = new HttpClient
        {
            BaseAddress = new Uri(AppendSlash(baseUrl)),
            Timeout = TimeSpan.FromSeconds(30),
        };
    }

    public void Dispose() => _http.Dispose();

    public Task<HealthResponse> HealthAsync(CancellationToken ct) => GetAsync<HealthResponse>("healthz", ct);

    public Task<BootstrapStatus> GetBootstrapStatusAsync(CancellationToken ct) => GetAsync<BootstrapStatus>("v1/bootstrap/status", ct);

    public Task EnsureBootstrapAsync(CancellationToken ct) => PostNoBodyAsync("v1/bootstrap/ensure", ct, requestTimeout: TimeSpan.FromHours(1));

    public Task<UpdateStatus> GetUpdateStatusAsync(CancellationToken ct) => GetAsync<UpdateStatus>("v1/update/status", ct);

    public Task CheckUpdatesAsync(CancellationToken ct) => PostNoBodyAsync("v1/update/check", ct, requestTimeout: TimeSpan.FromMinutes(5));

    public Task<ModelsResponse> GetModelsAsync(CancellationToken ct) => GetAsync<ModelsResponse>("v1/models", ct);

    public Task<PresetsResponse> GetPresetsAsync(CancellationToken ct) => GetAsync<PresetsResponse>("v1/models/presets", ct);

    public Task SetDefaultModelAsync(string model, CancellationToken ct) => PostJsonAsync("v1/models/use", new ModelUseRequest { Name = model }, ct);

    public Task InstallModelAsync(string modelName, CancellationToken ct) => PostJsonAsync("v1/models/install", new ModelInstallRequest { Name = modelName }, ct, requestTimeout: TimeSpan.FromHours(6));

    public Task<JobsResponse> GetJobsAsync(CancellationToken ct) => GetAsync<JobsResponse>("v1/jobs", ct);

    public Task AddJobAsync(AddJobRequest request, CancellationToken ct) => PostJsonAsync("v1/jobs", request, ct);

    public Task CancelJobAsync(string id, CancellationToken ct) => PostNoBodyAsync($"v1/jobs/{Uri.EscapeDataString(id)}/cancel", ct);

    public Task RetryJobAsync(string id, CancellationToken ct) => PostNoBodyAsync($"v1/jobs/{Uri.EscapeDataString(id)}/retry", ct);

    private async Task<T> GetAsync<T>(string path, CancellationToken ct)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, path);
        using var res = await _http.SendAsync(req, ct).ConfigureAwait(false);
        return await ReadOrThrowAsync<T>(res, ct).ConfigureAwait(false);
    }

    private async Task PostNoBodyAsync(string path, CancellationToken ct, TimeSpan? requestTimeout = null)
    {
        using var timeoutCts = CreateTimeoutCts(ct, requestTimeout);
        var effectiveToken = timeoutCts?.Token ?? ct;
        try
        {
            using var req = new HttpRequestMessage(HttpMethod.Post, path);
            using var res = await _http.SendAsync(req, effectiveToken).ConfigureAwait(false);
            await EnsureSuccessAsync(res, effectiveToken).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (IsTimeout(timeoutCts, ct))
        {
            throw new TimeoutException("Request timed out");
        }
    }

    private async Task PostJsonAsync<TReq>(string path, TReq body, CancellationToken ct, TimeSpan? requestTimeout = null)
    {
        using var timeoutCts = CreateTimeoutCts(ct, requestTimeout);
        var effectiveToken = timeoutCts?.Token ?? ct;
        try
        {
            using var res = await _http.PostAsJsonAsync(path, body, JsonOptions, effectiveToken).ConfigureAwait(false);
            await EnsureSuccessAsync(res, effectiveToken).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (IsTimeout(timeoutCts, ct))
        {
            throw new TimeoutException("Request timed out");
        }
    }

    private static async Task<T> ReadOrThrowAsync<T>(HttpResponseMessage response, CancellationToken ct)
    {
        if (!response.IsSuccessStatusCode)
        {
            throw await BuildApiError(response, ct).ConfigureAwait(false);
        }

        var value = await response.Content.ReadFromJsonAsync<T>(JsonOptions, ct).ConfigureAwait(false);
        if (value is null)
        {
            throw new InvalidOperationException("API returned empty response");
        }
        return value;
    }

    private static async Task EnsureSuccessAsync(HttpResponseMessage response, CancellationToken ct)
    {
        if (response.IsSuccessStatusCode)
        {
            return;
        }
        throw await BuildApiError(response, ct).ConfigureAwait(false);
    }

    private static async Task<Exception> BuildApiError(HttpResponseMessage response, CancellationToken ct)
    {
        string raw = string.Empty;
        try
        {
            raw = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
            var payload = JsonSerializer.Deserialize<ApiErrorResponse>(raw, JsonOptions);
            if (!string.IsNullOrWhiteSpace(payload?.Error))
            {
                return new InvalidOperationException(payload.Error);
            }
        }
        catch
        {
            // ignored
        }

        if (!string.IsNullOrWhiteSpace(raw))
        {
            return new InvalidOperationException(raw);
        }
        return new InvalidOperationException($"HTTP {(int)response.StatusCode} {response.ReasonPhrase}");
    }

    private static string AppendSlash(string url)
    {
        if (string.IsNullOrWhiteSpace(url))
        {
            throw new ArgumentException("Base URL is empty", nameof(url));
        }
        return url.EndsWith("/", StringComparison.Ordinal) ? url : url + "/";
    }

    private static CancellationTokenSource? CreateTimeoutCts(CancellationToken parent, TimeSpan? requestTimeout)
    {
        if (!requestTimeout.HasValue)
        {
            return null;
        }
        var cts = CancellationTokenSource.CreateLinkedTokenSource(parent);
        cts.CancelAfter(requestTimeout.Value);
        return cts;
    }

    private static bool IsTimeout(CancellationTokenSource? timeoutCts, CancellationToken parent)
    {
        return timeoutCts is not null && timeoutCts.IsCancellationRequested && !parent.IsCancellationRequested;
    }
}
