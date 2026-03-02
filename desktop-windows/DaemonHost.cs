using System;
using System.Collections.Concurrent;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;

namespace TranscribeDesktop;

public sealed class DaemonHost : IDisposable
{
    private readonly ConcurrentQueue<string> _logLines = new();
    private Process? _process;

    public string Addr { get; }
    public string BaseUrl { get; }
    public string? EnginePath { get; private set; }
    public bool StartedByApp { get; private set; }

    public DaemonHost(string addr = "127.0.0.1:9864")
    {
        Addr = addr;
        BaseUrl = $"http://{addr}";
    }

    public async Task StartAsync(CancellationToken ct)
    {
        if (await IsHealthyAsync(ct).ConfigureAwait(false))
        {
            StartedByApp = false;
            return;
        }

        EnginePath = ResolveEnginePath();
        if (string.IsNullOrWhiteSpace(EnginePath) || !File.Exists(EnginePath))
        {
            throw new FileNotFoundException("transcribe.exe was not found next to the app. Reinstall the release bundle.");
        }

        var psi = new ProcessStartInfo
        {
            FileName = EnginePath,
            WorkingDirectory = Path.GetDirectoryName(EnginePath) ?? AppContext.BaseDirectory,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true,
            WindowStyle = ProcessWindowStyle.Hidden,
        };
        psi.ArgumentList.Add("daemon");
        psi.ArgumentList.Add("run");
        psi.ArgumentList.Add("--addr");
        psi.ArgumentList.Add(Addr);

        var process = new Process { StartInfo = psi, EnableRaisingEvents = true };
        process.OutputDataReceived += (_, e) => EnqueueLog(e.Data);
        process.ErrorDataReceived += (_, e) => EnqueueLog(e.Data);

        if (!process.Start())
        {
            throw new InvalidOperationException("Failed to start transcribe.exe daemon");
        }

        process.BeginOutputReadLine();
        process.BeginErrorReadLine();
        _process = process;
        StartedByApp = true;

        var deadline = DateTime.UtcNow.AddSeconds(20);
        while (DateTime.UtcNow < deadline)
        {
            ct.ThrowIfCancellationRequested();

            if (process.HasExited)
            {
                throw new InvalidOperationException($"daemon exited with code {process.ExitCode}: {GetRecentLogs()}");
            }
            if (await IsHealthyAsync(ct).ConfigureAwait(false))
            {
                return;
            }
            await Task.Delay(250, ct).ConfigureAwait(false);
        }

        throw new TimeoutException($"daemon did not become healthy within 20 seconds: {GetRecentLogs()}");
    }

    public void Stop()
    {
        var process = _process;
        _process = null;
        if (process is null)
        {
            return;
        }

        try
        {
            if (!process.HasExited)
            {
                process.Kill(entireProcessTree: true);
                process.WaitForExit(3000);
            }
        }
        catch
        {
            // no-op
        }
        finally
        {
            process.Dispose();
        }
    }

    public string GetRecentLogs()
    {
        return string.Join(Environment.NewLine, _logLines.ToArray());
    }

    public void Dispose()
    {
        Stop();
    }

    private static string ResolveEnginePath()
    {
        var fromEnv = Environment.GetEnvironmentVariable("TRANSCRIBE_ENGINE_PATH");
        if (!string.IsNullOrWhiteSpace(fromEnv) && File.Exists(fromEnv))
        {
            return fromEnv;
        }

        var bundled = Path.Combine(AppContext.BaseDirectory, "transcribe.exe");
        if (File.Exists(bundled))
        {
            return bundled;
        }

        return string.Empty;
    }

    private async Task<bool> IsHealthyAsync(CancellationToken ct)
    {
        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(2) };
        try
        {
            using var res = await http.GetAsync(BaseUrl + "/healthz", ct).ConfigureAwait(false);
            return res.IsSuccessStatusCode;
        }
        catch
        {
            return false;
        }
    }

    private void EnqueueLog(string? line)
    {
        if (string.IsNullOrWhiteSpace(line))
        {
            return;
        }

        _logLines.Enqueue(line.Trim());
        while (_logLines.Count > 120 && _logLines.TryDequeue(out _))
        {
        }
    }
}
