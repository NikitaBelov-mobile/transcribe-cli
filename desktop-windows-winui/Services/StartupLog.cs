using System;
using System.IO;
using System.Text;

namespace TranscribeDesktop.WinUI.Services;

public static class StartupLog
{
    private static readonly object Sync = new();

    public static string LogFilePath
    {
        get
        {
            var root = AppServices.ResolveStateDirectory();
            var logsDir = Path.Combine(root, "logs");
            Directory.CreateDirectory(logsDir);
            return Path.Combine(logsDir, "desktop-startup.log");
        }
    }

    public static void Write(string message)
    {
        try
        {
            lock (Sync)
            {
                var line = $"{DateTimeOffset.UtcNow:O} | {message}{Environment.NewLine}";
                File.AppendAllText(LogFilePath, line, Encoding.UTF8);
            }
        }
        catch
        {
            // Best-effort logging only.
        }
    }

    public static void Write(Exception ex, string context)
    {
        Write($"{context}: {ex}");
    }
}
