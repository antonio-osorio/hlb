package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
)

var (
	// Lexer lexes HLB into tokens for the parser.
	Lexer = lexer.Must(regex.New(fmt.Sprintf(`
	        whitespace = [\r\t ]+

		Keyword  = \b(with|as|variadic)\b
		Type     = \b(string|int|bool|fs|option)(::[a-z][a-z]*)?\b
		Numeric  = \b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b
		Decimal  = \b(0|[1-9][0-9]*)\b
		String   = "(\\.|[^"])*"|'[^']*'
		Bool     = \b(true|false)\b
		Ident    = \b[a-zA-Z_][a-zA-Z0-9_]*\b
	        Newline  = \n
		Operator = {|}|\(|\)|,|;
	        Comment  = #[^\n]*\n
	        Bad      = .*
	`)))

	// Parser parses HLB into a concrete syntax tree rooted from a File.
	Parser = participle.MustBuild(
		&File{},
		participle.Lexer(Lexer),
		participle.Unquote(),
	)
)

// Node is implemented by all nodes in the CST.
type Node interface {
	// Stringer is implemented to unparse a node back into formatted HLB.
	fmt.Stringer

	// Position returns position of the first character belonging to the node.
	Position() lexer.Position

	// End returns position of the first character immediately after the node.
	End() lexer.Position
}

// File represents a HLB source file.
//
// Initially, the Parser will fill in this struct as a parse tree / concrete
// syntax tree, but a second pass from the Analyzer will type check and fill in
// fields without parser struct tags like scopes and doc linking.
type File struct {
	Pos   lexer.Position
	Scope *Scope
	Doc   *CommentGroup
	Decls []*Decl `( @@ )*`
}

func (f *File) Position() lexer.Position { return f.Pos }
func (f *File) End() lexer.Position {
	if len(f.Decls) == 0 {
		if f.Doc == nil {
			return shiftPosition(f.Pos, 1, 0)
		}

		return f.Doc.End()
	}
	return f.Decls[len(f.Decls)-1].End()
}

// Decl represents a declaration node.
type Decl struct {
	Pos     lexer.Position
	Bad     *BadDecl      `( @@`
	Import  *ImportDecl   `| @@`
	Export  *ExportDecl   `| @@`
	Func    *FuncDecl     `| @@`
	Newline *Newline      `| @@`
	Doc     *CommentGroup `| @@ )`
}

func (d *Decl) Position() lexer.Position { return d.Pos }
func (d *Decl) End() lexer.Position {
	switch {
	case d.Bad != nil:
		return d.Bad.End()
	case d.Import != nil:
		return d.Import.End()
	case d.Export != nil:
		return d.Export.End()
	case d.Func != nil:
		return d.Func.End()
	case d.Newline != nil:
		return d.Newline.End()
	case d.Doc != nil:
		return d.Doc.End()
	default:
		return shiftPosition(d.Pos, 1, 0)
	}
}

// BadDecl represents a declaration containing errors.
type BadDecl struct {
	Pos lexer.Position
	Bad string `@Bad`
}

func (d *BadDecl) Position() lexer.Position { return d.Pos }
func (d *BadDecl) End() lexer.Position      { return shiftPosition(d.Pos, len(d.Bad), 0) }

// ImportDecl represents an import declaration.
type ImportDecl struct {
	Pos    lexer.Position
	Ident  *Ident   `"import" @@`
	Import *FuncLit `"from" @@`
}

func (d *ImportDecl) Position() lexer.Position { return d.Pos }
func (d *ImportDecl) End() lexer.Position      { return d.Import.End() }

// ExportDecl represents an export declaration.
type ExportDecl struct {
	Pos   lexer.Position
	Ident *Ident `"export" @@`
}

func (d *ExportDecl) Position() lexer.Position { return d.Pos }
func (d *ExportDecl) End() lexer.Position      { return d.Ident.End() }

// FuncDecl represents a function declaration.
type FuncDecl struct {
	Pos    lexer.Position
	Scope *Scope
	Doc *CommentGroup
	Type   *Type      `@@`
	Method *Method    `( @@ )?`
	Name   *Ident     `@@`
	Params *FieldList `@@`
	Body   *BlockStmt `( @@ )?`
}

func (d *FuncDecl) Position() lexer.Position { return d.Pos }
func (d *FuncDecl) End() lexer.Position      { return d.Body.CloseBrace.End() }

type Method struct {
	Pos        lexer.Position
	OpenParen  *OpenParen  `@@`
	Type       *Type       `@@`
	CloseParen *CloseParen `@@`
}

func (m *Method) Position() lexer.Position { return m.Pos }
func (m *Method) End() lexer.Position      { return m.CloseParen.End() }

// FieldList represents a list of Fields, enclosed by parentheses.
type FieldList struct {
	Pos        lexer.Position
	OpenParen  *OpenParen  `@@`
	List       []*Field    `( ( Newline )? @@ ( "," ( Newline )?  @@ )* ( "," Newline )? )?`
	CloseParen *CloseParen `@@`
}

func (f *FieldList) Position() lexer.Position { return f.OpenParen.Pos }
func (f *FieldList) End() lexer.Position      { return f.CloseParen.End() }

// NumFields returns the number of fields in FieldList.
func (f *FieldList) NumFields() int {
	if f == nil {
		return 0
	}
	return len(f.List)
}

// Field represents a parameter declaration in a signature.
type Field struct {
	Pos      lexer.Position
	Variadic *Variadic `( @@ )?`
	Type     *Type     `@@`
	Name     *Ident    `@@`
}

func NewField(typ ObjType, name string, variadic bool) *Field {
	f := &Field{
		Type: NewType(typ),
		Name: NewIdent(name),
	}
	if variadic {
		f.Variadic = &Variadic{Keyword: "variadic"}
	}
	return f
}

func (f *Field) Position() lexer.Position { return f.Pos }
func (f *Field) End() lexer.Position      { return f.Name.End() }

type Variadic struct {
	Pos     lexer.Position
	Keyword string `@"variadic"`
}

func (v *Variadic) Position() lexer.Position { return v.Pos }
func (v *Variadic) End() lexer.Position      { return shiftPosition(v.Pos, len(v.Keyword), 0) }

// Expr represents an expression node.
type Expr struct {
	Pos      lexer.Position
	Ident    *Ident    `( @@`
	BasicLit *BasicLit `| @@`
	FuncLit  *FuncLit  `| @@ )`
}

func (e *Expr) Position() lexer.Position { return e.Pos }
func (e *Expr) End() lexer.Position {
	switch {
	case e.Ident != nil:
		return e.Ident.End()
	case e.BasicLit != nil:
		return e.BasicLit.End()
	case e.FuncLit != nil:
		return e.FuncLit.End()
	default:
		return shiftPosition(e.Pos, 1, 0)
	}
}

// Type represents an object type.
type Type struct {
	Pos     lexer.Position
	ObjType ObjType `@Type`
}

func (t *Type) Position() lexer.Position { return t.Pos }
func (t *Type) End() lexer.Position      { return shiftPosition(t.Pos, len(string(t.ObjType)), 0) }

func NewType(typ ObjType) *Type {
	return &Type{ObjType: typ}
}

func (t *Type) Type() ObjType {
	typeParts := strings.Split(string(t.ObjType), "::")
	return ObjType(typeParts[0])
}

func (t *Type) SubType() ObjType {
	typeParts := strings.Split(string(t.ObjType), "::")
	if len(typeParts) == 1 {
		return None
	}
	return ObjType(typeParts[1])
}

// Equals returns whether type equals another ObjType.
func (t *Type) Equals(typ ObjType) bool {
	return typ == t.Type()
}

type ObjType string

const (
	None           ObjType = ""
	Str            ObjType = "string"
	Int            ObjType = "int"
	Bool           ObjType = "bool"
	Filesystem     ObjType = "fs"
	Option         ObjType = "option"
	OptionImage    ObjType = "option::image"
	OptionHTTP     ObjType = "option::http"
	OptionGit      ObjType = "option::git"
	OptionLocal    ObjType = "option::local"
	OptionGenerate ObjType = "option::generate"
	OptionRun      ObjType = "option::run"
	OptionSSH      ObjType = "option::ssh"
	OptionSecret   ObjType = "option::secret"
	OptionMount    ObjType = "option::mount"
	OptionMkdir    ObjType = "option::mkdir"
	OptionMkfile   ObjType = "option::mkfile"
	OptionRm       ObjType = "option::rm"
	OptionCopy     ObjType = "option::copy"
)

// Ident represents an identifier.
type Ident struct {
	Pos  lexer.Position
	Name string `@Ident`
}

func (i *Ident) Position() lexer.Position { return i.Pos }
func (i *Ident) End() lexer.Position      { return shiftPosition(i.Pos, len(i.Name), 0) }

func NewIdent(name string) *Ident {
	return &Ident{Name: name}
}

func NewIdentExpr(name string) *Expr {
	return &Expr{
		Ident: NewIdent(name),
	}
}

// BasicLit represents a literal of basic type.
type BasicLit struct {
	Pos     lexer.Position
	Str     *string     `( @String`
	Decimal *int        `| @Decimal`
	Numeric *NumericLit `| @Numeric`
	Bool    *bool       `| @Bool )`
}

func (l *BasicLit) Position() lexer.Position { return l.Pos }
func (l *BasicLit) End() lexer.Position {
	switch {
	case l.Str != nil, l.Decimal != nil, l.Numeric != nil, l.Bool != nil:
		return shiftPosition(l.Pos, len(l.String()), 0)
	default:
		return shiftPosition(l.Pos, 1, 0)
	}
}

// NumericLit represents a number literal with a non-decimal base.
type NumericLit struct {
	Pos   lexer.Position
	Value int64
	Base  int
}

func (l *NumericLit) Position() lexer.Position { return l.Pos }
func (l *NumericLit) End() lexer.Position      { return shiftPosition(l.Pos, len(l.String()), 0) }

func (l *NumericLit) Capture(tokens []string) error {
	base := 10
	n := tokens[0]
	if len(n) >= 2 {
		switch n[1] {
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		case 'x', 'X':
			base = 16
		}
		n = n[2:]
	}
	var err error
	num, err := strconv.ParseInt(n, base, 64)
	l.Value = num
	l.Base = base
	return err
}

// ObjType returns the type of the basic literal.
func (l *BasicLit) ObjType() ObjType {
	switch {
	case l.Str != nil:
		return Str
	case l.Decimal != nil, l.Numeric != nil:
		return Int
	case l.Bool != nil:
		return Bool
	}
	panic("unknown basic lit")
}

func NewStringExpr(v string) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Str: &v,
		},
	}
}

func NewDecimalExpr(v int) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Decimal: &v,
		},
	}
}

func NewNumericExpr(v int64, base int) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Numeric: &NumericLit{
				Value: v,
				Base:  base,
			},
		},
	}
}

func NewBoolExpr(v bool) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Bool: &v,
		},
	}
}

// FuncLit represents a literal block prefixed by its type. If the type is
// missing then it's assumed to be a fs block literal.
type FuncLit struct {
	Pos  lexer.Position
	Type *Type      `@@`
	Body *BlockStmt `@@`
}

func (l *FuncLit) Position() lexer.Position { return l.Pos }
func (l *FuncLit) End() lexer.Position      { return l.Body.End() }

func (l *FuncLit) NumStmts() int {
	if l == nil {
		return 0
	}
	return l.Body.NumStmts()
}

func NewFuncLit(typ ObjType, stmts ...*Stmt) *FuncLit {
	return &FuncLit{
		Type: NewType(typ),
		Body: &BlockStmt{
			List: stmts,
		},
	}
}

func NewFuncLitExpr(typ ObjType, stmts ...*Stmt) *Expr {
	return &Expr{
		FuncLit: NewFuncLit(typ, stmts...),
	}
}

// Stmt represents a statement node.
type Stmt struct {
	Pos     lexer.Position
	Call    *CallStmt     `( @@`
	Newline *Newline      `| @@`
	Doc     *CommentGroup `| @@ )`
}

func (s *Stmt) Position() lexer.Position { return s.Pos }
func (s *Stmt) End() lexer.Position {
	switch {
	case s.Call != nil:
		return s.Call.End()
	case s.Newline != nil:
		return s.Newline.End()
	case s.Doc != nil:
		return s.Doc.End()
	default:
		return shiftPosition(s.Pos, 1, 0)
	}
}

// CallStmt represents an function name followed by an argument list, and an
// optional WithOpt.
type CallStmt struct {
	Pos     lexer.Position
	Doc     *CommentGroup
	Func    *Ident     `@@`
	Args    []*Expr    `( @@ )*`
	WithOpt *WithOpt   `( @@ )?`
	Alias   *AliasDecl `( @@ )?`
	StmtEnd *StmtEnd   `@@`
}

func (s *CallStmt) Position() lexer.Position { return s.Pos }
func (s *CallStmt) End() lexer.Position      { return s.StmtEnd.End() }

func NewCallStmt(name string, args []*Expr, withOpt *WithOpt, alias *AliasDecl) *Stmt {
	return &Stmt{
		Call: &CallStmt{
			Func:    NewIdent(name),
			Args:    args,
			WithOpt: withOpt,
			Alias:   alias,
		},
	}
}

// WithOpt represents optional arguments for a CallStmt.
type WithOpt struct {
	Pos     lexer.Position
	With    *With    `@@`
	Ident   *Ident   `( @@`
	FuncLit *FuncLit `| @@ )`
}

func (w *WithOpt) Position() lexer.Position { return w.Pos }
func (w *WithOpt) End() lexer.Position {
	switch {
	case w.Ident != nil:
		return w.Ident.End()
	case w.FuncLit != nil:
		return w.FuncLit.End()
	default:
		return shiftPosition(w.Pos, 1, 0)
	}
}

func NewWithIdent(name string) *WithOpt {
	return &WithOpt{
		With:  &With{Keyword: "with"},
		Ident: NewIdent(name),
	}
}

func NewWithFuncLit(stmts ...*Stmt) *WithOpt {
	return &WithOpt{
		With:    &With{Keyword: "with"},
		FuncLit: NewFuncLit(Option, stmts...),
	}
}

// With represents the keyword "with".
type With struct {
	Pos     lexer.Position
	Keyword string `@"with"`
}

func (w *With) Position() lexer.Position { return w.Pos }
func (w *With) End() lexer.Position      { return shiftPosition(w.Pos, len(w.Keyword), 0) }

// AliasDecl represents a declaration of an alias for a CallStmt.
type AliasDecl struct {
	Pos   lexer.Position
	As    *As    `@@`
	Ident *Ident `@@`
	Func  *FuncDecl
	Call  *CallStmt
}

func (d *AliasDecl) Position() lexer.Position { return d.Pos }
func (d *AliasDecl) End() lexer.Position      { return d.Ident.End() }

// As represents the keyword "as".
type As struct {
	Pos     lexer.Position
	Keyword string `@"as"`
}

func (a *As) Position() lexer.Position { return a.Pos }
func (a *As) End() lexer.Position      { return shiftPosition(a.Pos, len(a.Keyword), 0) }

// StmtEnd represents the end of a statement.
type StmtEnd struct {
	Pos       lexer.Position
	Semicolon *string  `( @";"`
	Newline   *Newline `| @@`
	Comment   *Comment `| @@ )`
}

func (e *StmtEnd) Position() lexer.Position { return e.Pos }
func (e *StmtEnd) End() lexer.Position {
	switch {
	case e.Semicolon != nil:
		return shiftPosition(e.Pos, len(*e.Semicolon), 0)
	case e.Newline != nil:
		return e.Newline.End()
	case e.Comment != nil:
		return e.Comment.End()
	default:
		return shiftPosition(e.Pos, 1, 0)
	}
}

// BlockStmt represents a braced statement list.
type BlockStmt struct {
	Pos        lexer.Position
	OpenBrace  *OpenBrace  `@@`
	List       []*Stmt     `( @@ )*`
	CloseBrace *CloseBrace `@@`
}

func (s *BlockStmt) Position() lexer.Position { return s.Pos }
func (s *BlockStmt) End() lexer.Position      { return s.CloseBrace.End() }

func (s *BlockStmt) NumStmts() int {
	if s == nil {
		return 0
	}
	num := 0
	for _, stmt := range s.List {
		if stmt.Newline != nil || stmt.Doc != nil {
			continue
		}
		num++
	}
	return num
}

func (s *BlockStmt) NonEmptyStmts() []*Stmt {
	if s == nil {
		return nil
	}
	var stmts []*Stmt
	for _, stmt := range s.List {
		if stmt.Newline != nil || stmt.Doc != nil {
			continue
		}
		stmts = append(stmts, stmt)
	}
	return stmts
}

// CommentGroup represents a sequence of comments with no other tokens and no
// empty lines between.
type CommentGroup struct {
	Pos  lexer.Position
	List []*Comment `( @@ )+`
}

func (g *CommentGroup) Position() lexer.Position { return g.Pos }
func (g *CommentGroup) End() lexer.Position {
	if len(g.List) == 0 {
		return shiftPosition(g.Pos, 1, 0)
	}
	return g.List[len(g.List)-1].End()
}

// NumComments returns the number of comments in CommentGroup.
func (g *CommentGroup) NumComments() int {
	if g == nil {
		return 0
	}
	return len(g.List)
}

// Comment represents a single comment.
type Comment struct {
	Pos  lexer.Position
	Text string `@Comment`
}

func (c *Comment) Position() lexer.Position { return c.Pos }
func (c *Comment) End() lexer.Position      { return shiftPosition(c.Pos, len(c.Text)-1, 0) }

type Newline struct {
	Pos  lexer.Position
	Text string `@Newline`
}

func (n *Newline) Position() lexer.Position { return n.Pos }
func (n *Newline) End() lexer.Position      { return shiftPosition(n.Pos, len(n.Text), 0) }

// OpenParen represents the "(" parenthese.
type OpenParen struct {
	Pos   lexer.Position
	Paren string `@"("`
}

func (p *OpenParen) Position() lexer.Position { return p.Pos }
func (p *OpenParen) End() lexer.Position      { return shiftPosition(p.Pos, 1, 0) }

// CloseParent represents the ")" parenthese.
type CloseParen struct {
	Pos   lexer.Position
	Paren string `@")"`
}

func (p *CloseParen) Position() lexer.Position { return p.Pos }
func (p *CloseParen) End() lexer.Position      { return shiftPosition(p.Pos, 1, 0) }

// OpenBrace represents the "{" brace.
type OpenBrace struct {
	Pos   lexer.Position
	Brace string `@"{"`
}

func (b *OpenBrace) Position() lexer.Position { return b.Pos }
func (b *OpenBrace) End() lexer.Position      { return shiftPosition(b.Pos, 1, 0) }

// CloseBrace represents the "}" brace.
type CloseBrace struct {
	Pos   lexer.Position
	Brace string `@"}"`
}

func (b *CloseBrace) Position() lexer.Position { return b.Pos }
func (b *CloseBrace) End() lexer.Position      { return shiftPosition(b.Pos, 1, 0) }

// Helper functions.

func shiftPosition(pos lexer.Position, offset int, line int) lexer.Position {
	pos.Offset += offset
	pos.Column += offset
	pos.Line += line
	return pos
}
