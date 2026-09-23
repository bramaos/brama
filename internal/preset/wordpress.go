package preset

import (
	"maps"

	"github.com/bramaos/brama/internal/config"
)

// wordpress is the Classification brama ships for a WordPress install.
//
// It covers the twelve tables a default single-site WordPress creates, and nothing
// else: a plugin's tables are the project's own to classify, and guessing at them from
// a name is the inference a Preset exists to replace.
//
// Table names are written against the prefix rather than against `wp_`. `wp_` is only
// the most common value of `$table_prefix` and not what it means — a hardened install is
// on `acme_` — so the prefix is read out of the project's own config by the Adapter and
// put in here. The keys core builds the same way, `$table_prefix` + `capabilities` in
// usermeta and `$table_prefix` + `user_roles` in options, are written against it too.
// See Lookup.
//
// The prefix is the site's own. A multisite install carries a second, per-site prefix on
// top of it, and those tables are the project's to classify: they are one site's copy of
// tables this already answers for, and naming them from here would be guessing at how
// many sites there are.
//
// Most of this is `keep`, and `keep` here is a statement about meaning and not a
// permission: an Environment receives none of these as real values until a human
// approves them, and an unapproved `keep` falls back to what a Generator would have
// given the column. See ADR 0010.
var wordpress = Preset{
	Name: "wordpress",
	Tables: map[string]config.Table{
		// The accounts table. Everything identifying a person is fabricated, and the
		// two columns holding the same login — WordPress derives `user_nicename` from
		// `user_login` — are correlated so they still match afterwards.
		"{prefix}users": columns(map[string]config.Column{
			"ID":            keep,
			"user_login":    correlated("username", "wp_user"),
			"user_nicename": correlated("username", "wp_user"),
			"user_pass":     fake("password"),
			"user_email":    correlated("email", "wp_email"),
			"user_url":      fake("url"),
			"display_name":  fake("full_name"),
			"user_status":   keep,
			// When the registration happened identifies nobody on its own, and a site
			// with every account registered at the same instant behaves strangely.
			"user_registered": keep,
			// A live password-reset token. Whoever holds it can take the account, and
			// it is worth nothing on a copy, so it travels as nothing.
			"user_activation_key": drop,
		}),

		// One of the three core tables that store many kinds of value in one column, so
		// it is classified per Discriminator value. The list is core's own keys: a key a
		// plugin writes is Unclassified until the project says what it holds, which is
		// the refusal working rather than a gap.
		"{prefix}usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys: with(map[string]config.Column{
				"first_name": fake("first_name"),
				"last_name":  fake("last_name"),
				"nickname":   fake("username"),
				// Free text the account holder wrote about themselves, published on the
				// site. Real data, and named as such rather than fabricated into
				// nonsense.
				"description": keep,
				// Live login sessions. A copy of these is a copy of everyone's
				// signed-in browser.
				"session_tokens": drop,
			}, keeps(
				"admin_color", "comment_shortcuts", "default_password_nag",
				"dismissed_wp_pointers", "locale", "primary_blog", "rich_editing",
				"show_admin_bar_front", "show_welcome_panel",
				"source_domain", "syntax_highlighting", "use_ssl",
				// The keys WordPress writes as `$table_prefix` + the name, so they are
				// `acme_capabilities` on an install prefixed `acme_`. They carry the
				// project's half of the name exactly as the table does.
				"{prefix}capabilities", "{prefix}dashboard_quick_press_last_post_id",
				"{prefix}user-settings", "{prefix}user-settings-time", "{prefix}user_level",
			)),
			Columns: keeps("umeta_id", "user_id"),
		},

		// Comments are written by the public, so the author block is every kind of
		// personal data a site collects without an account.
		"{prefix}comments": columns(with(map[string]config.Column{
			"comment_author":       fake("full_name"),
			"comment_author_email": correlated("email", "wp_email"),
			"comment_author_url":   fake("url"),
			// No Generator claims this by name — the column is spelled `IP`, and the
			// patterns are matched and not guessed at. The Preset knows the column, so
			// the Preset answers for it.
			"comment_author_IP": fake("ip"),
			// A browser fingerprint with no development value.
			"comment_agent": drop,
		}, keeps(
			"comment_ID", "comment_post_ID", "comment_date", "comment_date_gmt",
			"comment_content", "comment_karma", "comment_approved", "comment_type",
			"comment_parent", "user_id",
		))),
		"{prefix}commentmeta": columns(keeps("meta_id", "comment_id", "meta_key", "meta_value")),

		// Posts are the site's own content and travel whole. The exception is the
		// per-post password, which a Generator would otherwise claim by pattern and
		// fabricate a bcrypt hash into: WordPress stores that one in plain text and
		// compares it as plain text, so a hash there is a post nobody can open.
		"{prefix}posts": columns(with(map[string]config.Column{
			"post_password": drop,
		}, keeps(
			"ID", "post_author", "post_date", "post_date_gmt", "post_content",
			"post_title", "post_excerpt", "post_status", "comment_status", "ping_status",
			"post_name", "to_ping", "pinged", "post_modified", "post_modified_gmt",
			"post_content_filtered", "post_parent", "guid", "menu_order", "post_type",
			"post_mime_type", "comment_count",
		))),
		// The second key/value table. `meta_value` holds a thumbnail's ID, a nav menu
		// item's target and an oEmbed provider's cached HTML at once, so one answer for
		// the column is either a post library with no images or a cache of remote
		// responses travelling whole.
		"{prefix}postmeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys: with(map[string]config.Column{
				// The provider's HTML for an embedded URL, under a hash of that URL,
				// with `_oembed_time_<hash>` beside it. A remote response cached on the
				// way through, and refetched the first time the post is rendered.
				"_oembed_*": drop,
				// Who has the post open in the editor, as `timestamp:user_id`. On a copy
				// it is a lock nobody can release over an edit nobody is making.
				"_edit_lock": drop,
				// The archive WordPress built to answer a privacy request: a zip of
				// everything the site holds about one person, and the URL serving it.
				"_export_file_name": drop,
				"_export_file_url":  drop,
				// The image's generated sizes, and the EXIF block the camera wrote —
				// the credit line, the copyright, the moment of the shot. Real data
				// about the site's own media, and what every size on the page is cut
				// from: fabricate it and the library renders nothing. The two live in
				// one serialized value, so keeping the sizes keeps the EXIF with them.
				// `keep` is knowledge and not Approval, and this is one a destination
				// should be asked about rather than handed.
				"_wp_attachment_metadata": keep,
				// A link an editor typed into a nav menu, not a person's URL, so it is
				// the site's own content for the reason the blogroll is.
				"_menu_item_url": keep,
			}, keeps(
				"_edit_last", "_encloseme", "_pingme", "_thumbnail_id",
				"_wp_page_template", "_wp_desired_post_slug", "_wp_old_slug",
				"_wp_old_date", "_wp_trash_meta_status", "_wp_trash_meta_time",
				"_wp_trash_meta_comments_status",
				"_wp_attached_file", "_wp_attachment_image_alt",
				"_wp_attachment_backup_sizes", "_wp_attachment_context",
				"_wp_attachment_is_custom_background", "_wp_attachment_is_custom_header",
				// Core writes one of these per theme, `…_last_used_` + the theme's
				// directory, so the set is open by what the install has installed.
				"_wp_attachment_custom_header_last_used_*",
				"_menu_item_type", "_menu_item_menu_item_parent", "_menu_item_object",
				"_menu_item_object_id", "_menu_item_target", "_menu_item_classes",
				"_menu_item_xfn", "_menu_item_orphaned",
				"_customize_changeset_uuid", "_customize_restore_dismissed",
				"_wp_suggested_privacy_policy_content",
				"_wp_user_request_confirmed_timestamp",
				"_wp_user_request_completed_timestamp",
			)),
			Columns: keeps("meta_id", "post_id"),
		},

		// Site configuration, and the third key/value table. `option_value` holds
		// everything from the site title to the mail server's password, so a site whose
		// options were all fabricated does not boot and a site whose options were all
		// kept hands over working credentials.
		"{prefix}options": {
			Discriminator: "option_name",
			Value:         "option_value",
			Keys: with(map[string]config.Column{
				// The site's own address, not a person's. A fabricated one serves
				// nothing, and pointing it at the destination is the Pull's job rather
				// than Anonymization's.
				"siteurl": keep,
				"home":    keep,
				// The address WordPress mails from, and the one a pending change is
				// waiting on. A copy that keeps them mails the real owner every time
				// staging sends a notification.
				"admin_email":     fake("email"),
				"new_admin_email": fake("email"),
				// The mailbox posts-by-email collects from: an address, and the password
				// that opens it. WordPress stores that password in plain text and
				// compares it in plain text, so the hash `fake.password` fabricates is
				// not a password here — and a working mailbox credential is worth
				// nothing on a copy anyway.
				"mailserver_login": fake("email"),
				"mailserver_pass":  drop,
				// Host and username the filesystem updater logs in with, and the
				// password beside them.
				"ftp_credentials": drop,
				// Live recovery-mode keys. Whoever holds one is in the admin.
				"recovery_keys": drop,
				// Caches, in the one place WordPress keeps them that a Pull carries.
				// They regenerate on their own, and some hold a remote response — an
				// update check's answer, a feed body — that brought personal data in
				// with it. `_transient_timeout_*` is covered by the first of these.
				"_transient_*":      drop,
				"_site_transient_*": drop,
			}, keeps(
				"WPLANG", "active_plugins", "admin_email_lifespan",
				"auto_plugin_theme_update_emails", "auto_update_core_dev",
				"auto_update_core_major", "auto_update_core_minor",
				"auto_update_plugins", "auto_update_themes", "avatar_default",
				"avatar_rating", "blacklist_keys", "blog_charset", "blog_public",
				"blogdescription", "blogname", "can_compress_scripts", "category_base",
				"category_children", "close_comments_days_old",
				"close_comments_for_old_posts", "comment_max_links", "comment_moderation",
				"comment_order", "comment_previously_approved", "comment_registration",
				"comment_whitelist",
				"comments_notify", "comments_per_page", "cron", "current_theme",
				"date_format", "db_upgraded", "db_version", "default_category",
				"default_comment_status", "default_comments_page", "default_email_category",
				"default_link_category", "default_ping_status", "default_post_format",
				"default_role", "disallowed_keys", "finished_splitting_shared_terms",
				"fresh_site", "gmt_offset", "hack_file", "html_type",
				"https_detection_errors", "image_default_align", "image_default_link_type",
				"image_default_size", "initial_db_version", "large_size_h", "large_size_w",
				"link_manager_enabled", "links_updated_date_format", "mailserver_port",
				"mailserver_url", "medium_large_size_h", "medium_large_size_w",
				"medium_size_h", "medium_size_w", "moderation_keys", "moderation_notify",
				"nav_menu_options", "page_comments", "page_for_posts", "page_on_front",
				"permalink_structure", "ping_sites", "posts_per_page", "posts_per_rss",
				"recently_activated", "recently_edited", "recovery_mode_email_last_sent",
				"require_name_email", "rewrite_rules", "rss_use_excerpt", "show_avatars",
				"show_comments_cookies_opt_in", "show_on_front", "sidebars_widgets",
				"site_icon", "start_of_week", "sticky_posts", "stylesheet", "tag_base",
				"template", "theme_switched", "thread_comments", "thread_comments_depth",
				"thumbnail_crop", "thumbnail_size_h", "thumbnail_size_w", "time_format",
				"timezone_string", "uninstall_plugins", "upload_path", "upload_url_path",
				"uploads_use_yearmonth_folders", "use_balanceTags", "use_smilies",
				"use_trackback", "user_count", "users_can_register",
				// Core's own names, spelled `wp_` whatever the install is prefixed with.
				// These are not `$table_prefix` + a name and must not be written as one.
				"wp_attachment_pages_enabled", "wp_force_deactivated_plugins",
				"wp_page_for_privacy_policy",
				// Core names a widget's settings `widget_` + the widget's own id_base,
				// and every plugin widget is named the same way — so the core widgets
				// are written out one at a time rather than swept up by `widget_*`,
				// which would classify a plugin's settings as core's.
				"widget_archives", "widget_block", "widget_calendar",
				"widget_categories", "widget_custom_html", "widget_media_audio",
				"widget_media_gallery", "widget_media_image", "widget_media_video",
				"widget_meta", "widget_nav_menu", "widget_pages",
				"widget_recent-comments", "widget_recent-posts", "widget_rss",
				"widget_search", "widget_tag_cloud", "widget_text",
				// A theme's own modifications, `theme_mods_` + the theme's directory.
				// The prefix is open the way the install's themes directory is, and
				// what it holds is the Customizer settings an administrator saved.
				"theme_mods_*",
				// This one is `$table_prefix` + the name, as the usermeta keys are, so
				// it is `acme_user_roles` on an install prefixed `acme_`.
				"{prefix}user_roles",
			)),
			Columns: keeps("option_id", "autoload"),
		},

		// The blogroll. `link_url` matches a Generator pattern and is not a person's
		// URL: these are links the site's own editors wrote.
		"{prefix}links": columns(keeps(
			"link_id", "link_url", "link_name", "link_image", "link_target",
			"link_description", "link_visible", "link_owner", "link_rating",
			"link_updated", "link_rel", "link_notes", "link_rss",
		)),

		// Taxonomy. Categories and tags are site structure, and fabricating them
		// detaches every post from the terms it is filed under.
		"{prefix}terms":              columns(keeps("term_id", "name", "slug", "term_group")),
		"{prefix}termmeta":           columns(keeps("meta_id", "term_id", "meta_key", "meta_value")),
		"{prefix}term_taxonomy":      columns(keeps("term_taxonomy_id", "term_id", "taxonomy", "description", "parent", "count")),
		"{prefix}term_relationships": columns(keeps("object_id", "term_taxonomy_id", "term_order")),
	},
}

// The shorthands below exist so that the Classification above reads as a list of
// decisions rather than as a list of struct literals. A Preset is reviewed by someone
// asking "what does brama do to my users table?", and the answer has to be visible at a
// glance.

var (
	keep = config.Column{Action: config.Keep}
	drop = config.Column{Action: config.Drop}
)

func fake(generator string) config.Column {
	return config.Column{Action: config.Classification("fake." + generator)}
}

// correlated is a fabricated column sharing one identity with the others in its group,
// so a real value occurring in all of them becomes the same fabricated value in all of
// them and the joins between them survive.
func correlated(generator, group string) config.Column {
	return config.Column{Action: config.Classification("fake." + generator), Correlate: group}
}

func columns(cols map[string]config.Column) config.Table {
	return config.Table{Columns: cols}
}

func keeps(names ...string) map[string]config.Column {
	out := make(map[string]config.Column, len(names))
	for _, name := range names {
		out[name] = keep
	}
	return out
}

// with adds the plain keeps to the decided columns. The two are written separately
// because they are read differently: the decisions are the Preset, and the keeps are
// the rest of the table listed out so that nothing in it is left Unclassified.
func with(decided, kept map[string]config.Column) map[string]config.Column {
	out := maps.Clone(kept)
	maps.Copy(out, decided)
	return out
}
