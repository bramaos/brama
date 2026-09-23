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
// put in here. See Lookup.
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

		// The one core table that stores many kinds of value in one column, so it is
		// classified per Discriminator value. The list is core's own keys: a key a
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
				"source_domain", "syntax_highlighting", "use_ssl", "wp_capabilities",
				"wp_dashboard_quick_press_last_post_id", "wp_user-settings",
				"wp_user-settings-time", "wp_user_level",
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
		"{prefix}postmeta": columns(keeps("meta_id", "post_id", "meta_key", "meta_value")),

		// Site configuration. `option_value` holds everything from the site title to a
		// plugin's serialized settings, and a site whose options were fabricated does
		// not boot.
		"{prefix}options": columns(keeps("option_id", "option_name", "option_value", "autoload")),

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
