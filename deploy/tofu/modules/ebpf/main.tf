# Unheaded eBPF Module
# Manages: Whispering Void DaemonSet, eBPF collectors, BPF map volumes

variable "environment" { type = string }
variable "ring_buffer_size_mb" { type = number; default = 16 }
variable "rate_limit_events_per_sec" { type = number; default = 100 }
