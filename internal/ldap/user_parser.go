package ldap

import (
	"fmt"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
)

// Static errors.
var (
	ErrEntryNil             = fmt.Errorf("entry is nil")
	ErrUsernameRequired     = fmt.Errorf("username is required")
	ErrDNRequired           = fmt.Errorf("DN is required")
	ErrTimestampParseFailed = fmt.Errorf("unable to parse timestamp")
)

// UserParser handles parsing LDAP entries into User objects.
type UserParser struct {
	attributeMapping map[string]string
	logger           *zap.Logger
}

// User represents a parsed user from LDAP.
type User struct {
	DN           string            `json:"dn"`
	Username     string            `json:"username"`
	Email        string            `json:"email"`
	FirstName    string            `json:"first_name"`
	LastName     string            `json:"last_name"`
	DisplayName  string            `json:"display_name"`
	Groups       []string          `json:"groups"`
	Attributes   map[string]string `json:"attributes"`
	LastModified time.Time         `json:"last_modified"`
}

// NewUserParser creates a new user parser with default attribute mapping.
func NewUserParser(logger *zap.Logger) *UserParser {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &UserParser{
		attributeMapping: map[string]string{
			"username":     "sAMAccountName",
			"email":        "mail",
			"first_name":   "givenName",
			"last_name":    "sn",
			"display_name": "displayName",
			"groups":       "memberOf",
			"modified":     "whenChanged",
		},
		logger: logger,
	}
}

// SetAttributeMapping sets custom attribute mapping.
func (p *UserParser) SetAttributeMapping(mapping map[string]string) {
	p.attributeMapping = mapping
}

// ParseEntry parses an LDAP entry into a User object.
func (p *UserParser) ParseEntry(entry *ldap.Entry) (*User, error) {
	if entry == nil {
		return nil, ErrEntryNil
	}

	user := p.createUserStruct(entry.DN)
	p.parseAttributes(user, entry.Attributes)
	p.setDisplayNameIfEmpty(user)

	if err := p.validateUser(user, entry.DN); err != nil {
		return nil, err
	}

	p.logParsedUser(user)

	return user, nil
}

// createUserStruct creates an initialized User struct.
func (p *UserParser) createUserStruct(dn string) *User {
	return &User{
		DN:           dn,
		Username:     "",
		Email:        "",
		FirstName:    "",
		LastName:     "",
		DisplayName:  "",
		Groups:       make([]string, 0),
		Attributes:   make(map[string]string),
		LastModified: time.Time{},
	}
}

// parseAttributes parses LDAP attributes into the User struct.
func (p *UserParser) parseAttributes(user *User, attributes []*ldap.EntryAttribute) {
	for _, attr := range attributes {
		p.parseAttribute(user, attr)
	}
}

// parseAttribute parses a single LDAP attribute.
func (p *UserParser) parseAttribute(user *User, attr *ldap.EntryAttribute) {
	attrName := strings.ToLower(attr.Name)

	switch attrName {
	case p.attributeMapping["username"]:
		p.setStringValue(&user.Username, attr.Values)
	case p.attributeMapping["email"]:
		p.setStringValue(&user.Email, attr.Values)
	case p.attributeMapping["first_name"]:
		p.setStringValue(&user.FirstName, attr.Values)
	case p.attributeMapping["last_name"]:
		p.setStringValue(&user.LastName, attr.Values)
	case p.attributeMapping["display_name"]:
		p.setStringValue(&user.DisplayName, attr.Values)
	case p.attributeMapping["groups"]:
		user.Groups = attr.Values
	case p.attributeMapping["modified"]:
		p.setTimestampValue(&user.LastModified, attr.Values)
	default:
		p.setAttributeValue(user.Attributes, attr.Name, attr.Values)
	}
}

// setStringValue sets a string value from LDAP attribute values.
func (p *UserParser) setStringValue(target *string, values []string) {
	if len(values) > 0 {
		*target = values[0]
	}
}

// setTimestampValue sets a timestamp value from LDAP attribute values.
func (p *UserParser) setTimestampValue(target *time.Time, values []string) {
	if len(values) > 0 {
		if parsed, err := p.parseTimestamp(values[0]); err == nil {
			*target = parsed
		}
	}
}

// setAttributeValue sets a custom attribute value.
func (p *UserParser) setAttributeValue(attributes map[string]string, name string, values []string) {
	if len(values) > 0 {
		attributes[name] = values[0]
	}
}

// setDisplayNameIfEmpty sets display name if not provided.
func (p *UserParser) setDisplayNameIfEmpty(user *User) {
	if user.DisplayName == "" {
		user.DisplayName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
		user.DisplayName = strings.TrimSpace(user.DisplayName)
	}
}

// validateUser validates required user fields.
func (p *UserParser) validateUser(user *User, dn string) error {
	if user.Username == "" {
		return fmt.Errorf("%w but not found in entry: %s", ErrUsernameRequired, dn)
	}

	return nil
}

// logParsedUser logs the parsed user information.
func (p *UserParser) logParsedUser(user *User) {
	p.logger.Debug("parsed user entry",
		zap.String("dn", user.DN),
		zap.String("username", user.Username),
		zap.String("email", user.Email))
}

// ParseEntries parses multiple LDAP entries into User objects.
func (p *UserParser) ParseEntries(entries []*ldap.Entry) ([]*User, error) {
	users := make([]*User, 0, len(entries))

	var parseErrors []error

	for _, entry := range entries {
		user, err := p.ParseEntry(entry)
		if err != nil {
			p.logger.Warn("failed to parse entry", zap.String("dn", entry.DN), zap.Error(err))
			parseErrors = append(parseErrors, err)

			continue
		}

		users = append(users, user)
	}

	if len(parseErrors) > 0 {
		p.logger.Warn("some entries failed to parse",
			zap.Int("failed_count", len(parseErrors)),
			zap.Int("success_count", len(users)))
	}

	return users, nil
}

// parseTimestamp parses various timestamp formats commonly used in LDAP.
func (p *UserParser) parseTimestamp(timestamp string) (time.Time, error) {
	// Common LDAP timestamp formats
	formats := []string{
		"20060102150405.0Z",    // Active Directory format
		"20060102150405Z",      // Alternative AD format
		"2006-01-02T15:04:05Z", // ISO 8601
		"2006-01-02 15:04:05",  // Generic format
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, timestamp); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("%w: %s", ErrTimestampParseFailed, timestamp)
}

// FilterUsers filters users based on criteria.
func (p *UserParser) FilterUsers(users []*User, filter func(*User) bool) []*User {
	filtered := make([]*User, 0)

	for _, user := range users {
		if filter(user) {
			filtered = append(filtered, user)
		}
	}

	return filtered
}

// GetUserByDN finds a user by DN.
func (p *UserParser) GetUserByDN(users []*User, dn string) *User {
	for _, user := range users {
		if user.DN == dn {
			return user
		}
	}

	return nil
}

// GetUserByUsername finds a user by username.
func (p *UserParser) GetUserByUsername(users []*User, username string) *User {
	for _, user := range users {
		if strings.EqualFold(user.Username, username) {
			return user
		}
	}

	return nil
}

// ValidateUser validates that a user has required fields.
func (p *UserParser) ValidateUser(user *User) error {
	if user.DN == "" {
		return ErrDNRequired
	}

	if user.Username == "" {
		return ErrUsernameRequired
	}

	return nil
}
