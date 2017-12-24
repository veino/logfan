// Code generated by "bitfanDoc -codec encoder"; DO NOT EDIT
package plaincodec

import "github.com/vjeantet/bitfan/processors/doc"

func Doc() *doc.Codec {
	return &doc.Codec{
  Name:       "encoder",
  PkgName:    "plaincodec",
  ImportPath: "github.com/vjeantet/bitfan/codecs/plain",
  Doc:        "doc codec",
  DocShort:   "",
  Decoder:    &doc.Decoder{
    Doc:     "",
    Options: &doc.CodecOptions{
      Doc:     "",
      Options: []*doc.CodecOption{},
    },
  },
  Encoder: &doc.Encoder{
    Doc:     "doc encoder",
    Options: &doc.CodecOptions{
      Doc:     "doc encoderOptions",
      Options: []*doc.CodecOption{
        &doc.CodecOption{
          Name:           "Format",
          Alias:          "format",
          Doc:            "Format as a golang html/template",
          Required:       false,
          Type:           "location",
          DefaultValue:   "\"{{.message}}\"",
          PossibleValues: []string{},
          ExampleLS:      "",
        },
        &doc.CodecOption{
          Name:           "Var",
          Alias:          "var",
          Doc:            "You can set variable to be used in Statements by using ${var}.\neach reference will be replaced by the value of the variable found in Statement's content\nThe replacement is case-sensitive.",
          Required:       false,
          Type:           "hash",
          DefaultValue:   nil,
          PossibleValues: []string{},
          ExampleLS:      "var => {\"hostname\"=>\"myhost\",\"varname\"=>\"varvalue\"}",
        },
      },
    },
  },
}
}