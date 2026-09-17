<?php
/**
 * The rows a Sync has to carry, and the ones it has to refuse.
 *
 * Run by seed.sh through `wp eval-file`, which is one WordPress bootstrap for the
 * whole seed rather than one per row — the difference between seconds and minutes.
 *
 * Everything here is derived from the row index. No clock, no randomness beyond a
 * fixed seed, no auto-increment left to chance: seeding twice must produce the same
 * database, or a test that compares before and after a Pull is comparing noise.
 */

// No `declare(strict_types=1)` and no top-level `const`: `wp eval-file` strips the
// opening tag and runs the rest through eval(), where a declare is a fatal error and
// a compile-time constant is not worth the risk.

if (!defined('ABSPATH')) {
	fwrite(STDERR, "seed.php must be run through `wp eval-file`\n");
	exit(1);
}

/** @var array<int, string> $args positional arguments from wp eval-file */
$userCount    = (int) ($args[0] ?? 12);
$productCount = (int) ($args[1] ?? 8);
$orderCount   = (int) ($args[2] ?? 5);

mt_srand(1337);

// Creating an order fires WooCommerce's customer emails, and the Server has no MTA — so
// every one of them prints a sendmail error and takes a timeout doing it. Short-
// circuiting wp_mail is also one less thing that differs between two seedings.
add_filter('pre_wp_mail', '__return_true');

/**
 * A date derived from an index, so the same row always carries the same timestamp.
 * The stride is prime so consecutive rows do not land on the same hour.
 */
function seeded_date($index)
{
	return gmdate('Y-m-d H:i:s', strtotime('2024-01-01 00:00:00 UTC') + $index * 3607);
}

$firstNames = ['Ada', 'Grace', 'Alan', 'Katherine', 'Linus', 'Radia', 'Edsger', 'Barbara', 'Ken', 'Margaret'];
$lastNames  = ['Lovelace', 'Hopper', 'Turing', 'Johnson', 'Torvalds', 'Perlman', 'Dijkstra', 'Liskov', 'Thompson', 'Hamilton'];
$cities     = ['Lviv', 'Kraków', 'Porto', 'Tallinn', 'Valencia', 'Utrecht'];
$countries  = ['UA', 'PL', 'PT', 'EE', 'ES', 'NL'];

/*
 * Users, and the wp_usermeta rows that make wp_usermeta.meta_key a Discriminator.
 *
 * The point of the spread is that one column holds values needing different
 * Classifications: billing_email is personal and would be faked, billing_country is
 * not and would be kept, session_tokens is neither and would be dropped. A rig whose
 * usermeta held three keys could not tell those cases apart.
 */
echo "users\n";
for ($i = 0; $i < $userCount; $i++) {
	$first = $firstNames[$i % count($firstNames)];
	$last  = $lastNames[intdiv($i, count($firstNames)) % count($lastNames)];
	$login = sprintf('customer%03d', $i + 1);

	$userId = wp_insert_user([
		'user_login'      => $login,
		'user_pass'       => 'brama-testenv',
		'user_email'      => sprintf('%s@brama-testenv.invalid', $login),
		'user_nicename'   => $login,
		'display_name'    => "$first $last",
		'first_name'      => $first,
		'last_name'       => $last,
		'role'            => 'customer',
		'user_registered' => seeded_date($i),
	]);

	if (is_wp_error($userId)) {
		fwrite(STDERR, "user $login: " . $userId->get_error_message() . "\n");
		exit(1);
	}

	$city    = $cities[$i % count($cities)];
	$country = $countries[$i % count($countries)];

	foreach ([
		'nickname'                => $login,
		'description'             => "Seeded customer $login.",
		'billing_first_name'      => $first,
		'billing_last_name'       => $last,
		'billing_company'         => sprintf('%s Holdings', $last),
		'billing_address_1'       => sprintf('%d %s Street', 10 + $i, $last),
		'billing_city'            => $city,
		'billing_postcode'        => sprintf('%05d', 10000 + $i * 7),
		'billing_country'         => $country,
		'billing_email'           => sprintf('%s@brama-testenv.invalid', $login),
		'billing_phone'           => sprintf('+380 44 %03d %04d', $i, 1000 + $i),
		'shipping_first_name'     => $first,
		'shipping_last_name'      => $last,
		'shipping_address_1'      => sprintf('%d %s Street', 10 + $i, $last),
		'shipping_city'           => $city,
		'shipping_country'        => $country,
		'session_tokens'          => 'a:0:{}',
		'wc_last_active'          => (string) strtotime(seeded_date($i)),
		'_woocommerce_persistent_cart_1' => 'a:0:{}',
		'last_update'             => (string) strtotime(seeded_date($i)),
	] as $key => $value) {
		update_user_meta($userId, $key, $value);
	}
}

echo "products\n";
$productIds = [];
for ($i = 0; $i < $productCount; $i++) {
	$product = new WC_Product_Simple();
	$product->set_name(sprintf('Seeded Product %02d', $i + 1));
	$product->set_slug(sprintf('seeded-product-%02d', $i + 1));
	$product->set_sku(sprintf('BRAMA-%04d', $i + 1));
	// Parenthesised around the division: a cast binds tighter than `/`, so
	// `(string) (900 + $i * 250) / 100` would divide a string and hand WooCommerce a
	// float where it documents a string.
	$product->set_regular_price((string) ((900 + $i * 250) / 100));
	$product->set_description(sprintf('Deterministic seed product number %d.', $i + 1));
	$product->set_short_description('Seeded by the brama testenv rig.');
	$product->set_manage_stock(true);
	$product->set_stock_quantity(10 + $i);
	$product->set_catalog_visibility('visible');
	$product->set_status('publish');
	$product->set_date_created(seeded_date($i));
	$productIds[] = $product->save();
}

echo "orders\n";
$statuses = ['completed', 'processing', 'on-hold', 'refunded'];
for ($i = 0; $i < $orderCount; $i++) {
	$customer = get_user_by('login', sprintf('customer%03d', ($i % $userCount) + 1));

	$order = wc_create_order(['customer_id' => $customer->ID]);
	$order->add_product(wc_get_product($productIds[$i % count($productIds)]), 1 + ($i % 3));
	$order->set_address([
		'first_name' => get_user_meta($customer->ID, 'billing_first_name', true),
		'last_name'  => get_user_meta($customer->ID, 'billing_last_name', true),
		'email'      => get_user_meta($customer->ID, 'billing_email', true),
		'phone'      => get_user_meta($customer->ID, 'billing_phone', true),
		'address_1'  => get_user_meta($customer->ID, 'billing_address_1', true),
		'city'       => get_user_meta($customer->ID, 'billing_city', true),
		'postcode'   => get_user_meta($customer->ID, 'billing_postcode', true),
		'country'    => get_user_meta($customer->ID, 'billing_country', true),
	], 'billing');
	$order->set_date_created(seeded_date($i));
	$order->calculate_totals();
	$order->set_status($statuses[$i % count($statuses)]);
	$order->save();
}

/*
 * The planted Unclassified column.
 *
 * An Unclassified column is one found in the source with no Classification, and it
 * causes a Refusal. Presets will eventually cover every column WordPress and
 * WooCommerce ship, so a rig built only from those could never produce one — and the
 * Refusal path would go untested precisely because the seed was tidy.
 *
 * This is the untidiness a real site has: a column some plugin or agency added years
 * ago, holding something that looks like personal data, that nothing knows about.
 */
echo "unclassified column\n";
global $wpdb;

$exists = $wpdb->get_var(
	$wpdb->prepare(
		"SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = %s AND column_name = %s",
		$wpdb->users,
		'legacy_crm_reference'
	)
);

if (!$exists) {
	$wpdb->query("ALTER TABLE {$wpdb->users} ADD COLUMN legacy_crm_reference VARCHAR(191) NULL");
}

$wpdb->query(
	"UPDATE {$wpdb->users}
	 SET legacy_crm_reference = CONCAT('CRM-', LPAD(ID, 6, '0'), '-', SUBSTRING(MD5(user_login), 1, 8))"
);

/*
 * WordPress stamps its own install artifacts with the clock: the admin account, the
 * sample post and page, the pages WooCommerce creates on activation. Two seedings an
 * hour apart would then differ in rows nobody seeded, and a test that compares a dump
 * taken before a Pull with one taken after would be comparing the clock.
 *
 * What is left clock-bound after this is wp_options — transients and the cron
 * schedule — and wp_users.user_pass, which WordPress salts. README.md says so.
 */
echo "normalising install timestamps\n";
$fixed = seeded_date(0);

$wpdb->query($wpdb->prepare(
	"UPDATE {$wpdb->users} SET user_registered = %s WHERE user_login = 'admin'",
	$fixed
));
$wpdb->query($wpdb->prepare(
	"UPDATE {$wpdb->posts} SET post_date = %s, post_date_gmt = %s
	 WHERE post_type NOT IN ('product', 'shop_order')",
	$fixed,
	$fixed
));
$wpdb->query("UPDATE {$wpdb->posts} SET post_modified = post_date, post_modified_gmt = post_date_gmt");
$wpdb->query($wpdb->prepare(
	"UPDATE {$wpdb->comments} SET comment_date = %s, comment_date_gmt = %s",
	$fixed,
	$fixed
));
$wpdb->query(
	"DELETE FROM {$wpdb->options}
	 WHERE option_name LIKE '\_transient\_%' OR option_name LIKE '\_site\_transient\_%'"
);

// WooCommerce stamps these two with the clock every time it saves a customer, which
// creating an order does. Setting them in the user loop above is not enough — they
// have to be set again here, after the last save, or the seed drifts by however many
// seconds the orders took.
$wpdb->query(
	"UPDATE {$wpdb->usermeta} m
	 JOIN {$wpdb->users} u ON u.ID = m.user_id
	 SET m.meta_value = UNIX_TIMESTAMP(u.user_registered)
	 WHERE m.meta_key IN ('wc_last_active', 'last_update')"
);

// Order keys are generated from wp_generate_password, and the paid/completed stamps
// come from the clock at the moment the status was set. Both are derived from the
// order instead, which keeps them plausible and makes them repeatable.
$wpdb->query(
	"UPDATE {$wpdb->postmeta} m
	 JOIN {$wpdb->posts} p ON p.ID = m.post_id
	 SET m.meta_value = CONCAT('wc_order_', LPAD(p.ID, 13, '0'))
	 WHERE m.meta_key = '_order_key'"
);
$wpdb->query(
	"UPDATE {$wpdb->postmeta} m
	 JOIN {$wpdb->posts} p ON p.ID = m.post_id
	 SET m.meta_value = p.post_date
	 WHERE m.meta_key IN ('_completed_date', '_paid_date')"
);
$wpdb->query(
	"UPDATE {$wpdb->postmeta} m
	 JOIN {$wpdb->posts} p ON p.ID = m.post_id
	 SET m.meta_value = UNIX_TIMESTAMP(p.post_date)
	 WHERE m.meta_key IN ('_date_completed', '_date_paid')"
);

echo "done\n";
