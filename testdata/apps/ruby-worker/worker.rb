#!/usr/bin/env ruby
# Long-running Ruby worker with SIGTERM handling via Signal.trap.
# $stdout.sync = true disables Ruby's default line buffering so the
# supervisor's log tail matches `lynxpm logs --follow` output ordering.

$stdout.sync = true

Signal.trap("TERM") do
  puts "ruby-worker received SIGTERM, exiting"
  exit 0
end
Signal.trap("INT") do
  puts "ruby-worker received SIGINT, exiting"
  exit 0
end

puts "ruby-worker pid=#{Process.pid}"
tick = 0
loop do
  puts "ruby-worker tick=#{tick}"
  tick += 1
  sleep 1
end
