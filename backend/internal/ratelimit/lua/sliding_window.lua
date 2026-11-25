local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local nonce = ARGV[3]

local t = redis.call("TIME")
local now = t[1] * 1000 + math.floor(t[2] / 1000)

redis.call("ZREMRANGEBYSCORE", key, "-inf", now - window_ms)
local count = redis.call("ZCARD", key)

if count >= limit then
	local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
	return {0, 0, tonumber(oldest[2]) + window_ms}
end

redis.call("ZADD", key, now, nonce)
redis.call("PEXPIRE", key, window_ms)

local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
return {1, limit - count - 1, tonumber(oldest[2]) + window_ms}
