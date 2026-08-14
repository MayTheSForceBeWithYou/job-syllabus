package store

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestCursorRoundTrip(t *testing.T) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "POSTING#abc123"},
		"SK": &types.AttributeValueMemberS{Value: "META"},
	}

	cursor, err := EncodeCursor(key)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	if cursor == "" {
		t.Fatal("EncodeCursor returned empty string for a non-empty key")
	}

	decoded, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if len(decoded) != len(key) {
		t.Fatalf("decoded key has %d attributes, want %d", len(decoded), len(key))
	}
	for k, want := range key {
		got, ok := decoded[k].(*types.AttributeValueMemberS)
		if !ok {
			t.Fatalf("decoded[%q] is not a string attribute: %T", k, decoded[k])
		}
		if got.Value != want.(*types.AttributeValueMemberS).Value {
			t.Errorf("decoded[%q] = %q, want %q", k, got.Value, want.(*types.AttributeValueMemberS).Value)
		}
	}
}

func TestEncodeCursorEmptyKey(t *testing.T) {
	cursor, err := EncodeCursor(nil)
	if err != nil {
		t.Fatalf("EncodeCursor(nil): %v", err)
	}
	if cursor != "" {
		t.Errorf("EncodeCursor(nil) = %q, want empty string", cursor)
	}
}

func TestDecodeCursorEmptyString(t *testing.T) {
	key, err := DecodeCursor("")
	if err != nil {
		t.Fatalf("DecodeCursor(\"\"): %v", err)
	}
	if key != nil {
		t.Errorf("DecodeCursor(\"\") = %v, want nil", key)
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	if _, err := DecodeCursor("not-valid-base64!!!"); err == nil {
		t.Error("DecodeCursor accepted invalid base64 without error")
	}
}
