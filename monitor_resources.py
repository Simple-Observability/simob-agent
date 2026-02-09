import re
import os
import sys
import math
import time
import shlex
import argparse
import subprocess
import statistics
import threading


def get_process_uptime(pid):
  # Get process start time (relative to system start time)
  with open(f"/proc/{pid}/stat", "r") as stat:
    data = stat.read().split(" ")
  clock_ticks = os.sysconf(os.sysconf_names['SC_CLK_TCK'])
  starttime = int(data[21]) / clock_ticks
  # Get system uptime
  with open("/proc/uptime", "r") as f:
    system_uptime = float(f.read().split()[0])
  # Return process uptime
  return system_uptime - starttime


def get_cpu_ticks(pid):
  "Format: https://man7.org/linux/man-pages/man5/proc_pid_stat.5.html"
  with open(f"/proc/{pid}/stat", "r") as stat:
    data = stat.read().split(" ")
  utime = int(data[13])
  stime = int(data[14])
  return utime + stime


def get_mem_uss(pid):
  "Format: https://www.kernel.org/doc/Documentation/ABI/testing/procfs-smaps_rollup"
  with open(f"/proc/{pid}/smaps_rollup", "r") as smaps:
    matches = re.findall(
      r"^(?:Private_Clean|Private_Dirty):\s+(\d+)\s+kB",
      smaps.read(), re.MULTILINE
    )
  return sum(int(m) for m in matches)


def get_disk_bytes(pid):
  "Format: https://man7.org/linux/man-pages/man5/proc_pid_io.5.html"
  with open(f"/proc/{pid}/io", "r") as io:
    content = io.read()
  for line in content.split("\n"):
    if len(line) == 0: continue
    entry = line.split(": ")
    if entry[0] == "read_bytes":
      read_bytes = int(entry[1])
      continue
    if entry[0] == "write_bytes":
      write_bytes = int(entry[1])
      continue

  return read_bytes, write_bytes


def lifetime_stats(pid):
  print(f"--- Lifetime Stats for PID {pid} ---")

  process_uptime = get_process_uptime(pid)
  print(f"Uptime: {process_uptime:.0f} seconds ({process_uptime / 3600:.1f}) hours")

  clock_ticks = os.sysconf(os.sysconf_names['SC_CLK_TCK'])
  total_ticks = get_cpu_ticks(pid)
  cpu_util = (total_ticks / clock_ticks) / process_uptime
  print("CPU")
  print(f"  Utilization (avg): {cpu_util*100:.2f} %")

  uss = get_mem_uss(args.pid)
  print("Memory:")
  print(f"  Current: {uss} kB")

  r_bytes, w_bytes = get_disk_bytes(pid)
  print("Disk I/O:")
  print(f"  Read: {r_bytes / 1000} kB ({r_bytes / process_uptime:.0f} B/s)")
  print(f"  Write: {w_bytes / 1000} kB ({w_bytes / process_uptime:.0f} B/s)")


def run_and_measure(command, poll_hz=50):
  def monitor_resources():
    nonlocal start_cpu_ticks, end_cpu_ticks
    nonlocal start_disk_read, start_disk_write, end_disk_read, end_disk_write
    try:
      # Initial measurements
      start_cpu_ticks = get_cpu_ticks(pid)
      start_disk_read, start_disk_write = get_disk_bytes(pid)

      while not process_finished:
        try:
          # Sample memory usage
          uss = get_mem_uss(pid)
          memory_samples.append(uss)

          # Overwrite final measurements
          end_cpu_ticks = get_cpu_ticks(pid)
          end_disk_read, end_disk_write = get_disk_bytes(pid)

          time.sleep(poll_interval)
        except (FileNotFoundError, ProcessLookupError):
          # Process ended
          break

    except (FileNotFoundError, ProcessLookupError):
      raise RuntimeError("Process ended before start of monitoring")

  # Variables to store measurements
  start_cpu_ticks = None
  end_cpu_ticks = None
  start_disk_read = None
  start_disk_write = None
  end_disk_read = None
  end_disk_write = None
  memory_samples = []

  print(f"Running command: {command}")

  # Start the process
  process_finished = False
  start_time = time.time()
  proc = subprocess.Popen(
    shlex.split(command),
    shell=False,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
  )
  pid = proc.pid
  print("Program has PID", pid)

  # Start monitoring thread
  poll_interval = 1.0 / poll_hz
  monitor_thread = threading.Thread(target=monitor_resources, daemon=True)
  monitor_thread.start()

  # Wait for process to finish
  stdout, stderr = proc.communicate()
  process_finished = True
  end_time = time.time()

  # Wait for monitoring thread to finish
  monitor_thread.join(timeout=1.0)

  # Not perfectly precise: there's a small delay on start/stop,
  # but the offsets maybe cancel out. Good enough for this use case,
  # we're not doing microbenchmarking
  runtime = end_time - start_time

  # Compute stats
  print("\n----- Results -----")
  # CPU
  clock_ticks = os.sysconf(os.sysconf_names['SC_CLK_TCK'])
  cpu_time = (end_cpu_ticks - start_cpu_ticks) / clock_ticks
  cpu_util = (cpu_time / runtime) * 100
  print("# CPU")
  print(f"Utilization: {cpu_util:.2f} %")
  print()

  # Memory
  print(f"# Memory statistics ({len(memory_samples)} samples):")
  print(f"Median: {statistics.median(memory_samples):.0f} kB")
  print(f"p95: {statistics.quantiles(memory_samples, n=100)[94]:.0f} kB")
  print(f"Max: {max(memory_samples):.0f} kB")
  print()

  # Disk
  read_bytes = end_disk_read - start_disk_read
  write_bytes = end_disk_write - start_disk_write
  print(f"# Disk I/O:")
  print(f"Read: {read_bytes / 1000:.0f} kB ({read_bytes / runtime:.0f} B/s)")
  print(f"Write: {write_bytes / 1000:.0f} kB ({write_bytes / runtime:.0f} B/s)")
  print("-------------------\n")


if __name__ == "__main__":
  parser = argparse.ArgumentParser()
  group = parser.add_mutually_exclusive_group(required=True)
  group.add_argument("--pid", help="Attach to running PID")
  group.add_argument("--run", help="Launch and monitor a command")

  args = parser.parse_args()

  if args.pid is not None:
    lifetime_stats(args.pid)
  elif args.run:
    run_and_measure(args.run)

