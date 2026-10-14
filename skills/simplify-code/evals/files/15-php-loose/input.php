<?php
// legacy ACL helpers (PHP 8.2)
function is_allowed($role, $allowed) {
    if (empty($role)) return false;
    if (in_array($role, $allowed)) return true;
    return false;
}

function active_ids($rows) {
    $out = array();
    foreach ($rows as $k => $r) {
        if ($r['active'] == true) {
            $out[$k] = $r['id'];
        }
    }
    return $out;
}
