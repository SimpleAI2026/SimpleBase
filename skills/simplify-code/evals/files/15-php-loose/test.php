<?php
// usage: php test.php <impl.php>
require $argv[1];
function check($c, $m) { if (!$c) { echo "FAIL $m\n"; exit(1); } }
check(is_allowed("0", ["0"]) === false, 'empty("0") is falsy -> false');
check(is_allowed("", [""]) === false, 'empty string -> false');
check(is_allowed(null, [null]) === false, 'null -> false');
check(is_allowed("1", [1]) === true, 'loose in_array "1" vs int 1');
check(is_allowed("admin", ["admin", "x"]) === true, 'basic');
check(is_allowed(1.0, ["1"]) === true, 'float vs string loose');
check(is_allowed("editor", ["admin"]) === false, 'not allowed');
$rows = [5 => ['active' => 'yes', 'id' => 'a'], 7 => ['active' => 0, 'id' => 'b'], 9 => ['active' => 1, 'id' => 'c'], 11 => ['active' => '0', 'id' => 'd'], 12 => ['active' => true, 'id' => 'e']];
check(active_ids($rows) === [5 => 'a', 9 => 'c', 12 => 'e'], 'loose ==true and preserved keys: ' . json_encode(active_ids($rows)));
check(active_ids([]) === [], 'empty');
echo "PASS\n";
