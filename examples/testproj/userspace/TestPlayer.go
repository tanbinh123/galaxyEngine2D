/*
All user defined 2D objects should be put here.
*/

package objs

import (
	"fmt"

	"galaxyzeta.io/engine/component"
	"galaxyzeta.io/engine/core"
	"galaxyzeta.io/engine/graphics"
	"galaxyzeta.io/engine/input"
	"galaxyzeta.io/engine/input/keys"
	"galaxyzeta.io/engine/linalg"
	"galaxyzeta.io/engine/sdk"
)

// TestPlayer is a golang GameObject2D testing template,
// It illustrates how to use Galaxy2DEngine.
type TestPlayer struct {
	*core.GameObject2D
	tf *component.Transform2D
}

//TestPlayer_OnCreate is a public constructor.
func TestPlayer_OnCreate() core.IGameObject2D {
	fmt.Println("SDK Call onCreate")
	gameObject2D := core.NewGameObject2D().
		RegisterRender(__TestPlayer_OnRender).
		RegisterStep(__TestPlayer_OnStep).
		RegisterDestroy(__TestPlayer_OnDestroy)
	gameObject2D.Sprite = graphics.NewSpriteInstance("spr_megaman")
	return &TestPlayer{
		GameObject2D: gameObject2D,
		tf:           gameObject2D.GetComponent(component.NameTransform2D).(*component.Transform2D),
	}
}

//__TestPlayer_OnStep is intentionally names with two underlines,
// telling user never call this function in other functions, that will not work,
// even damaging the whole game logic.
func __TestPlayer_OnStep(obj core.IGameObject2D) {
	this := obj.(*TestPlayer)
	isKeyHeld := false
	if input.IsKeyHeld(keys.KeyW) {
		this.tf.Y -= 1
		isKeyHeld = true
	} else if input.IsKeyHeld(keys.KeyS) {
		this.tf.Y += 1
		isKeyHeld = true
	}
	if input.IsKeyHeld(keys.KeyA) {
		this.tf.X -= 1
		isKeyHeld = true
	} else if input.IsKeyHeld(keys.KeyD) {
		this.tf.X += 1
		isKeyHeld = true
	}
	if isKeyHeld {
		this.Sprite.EnableAnimation()
	} else {
		this.Sprite.DisableAnimation()
	}
}

func __TestPlayer_OnRender(obj core.IGameObject2D) {
	this := obj.(*TestPlayer)
	this.Sprite.Render(sdk.GetCamera(0), linalg.Point2f32{X: this.tf.X, Y: this.tf.Y})
}

func __TestPlayer_OnDestroy(obj core.IGameObject2D) {
	// this := obj.(*TestInputDetection)
	fmt.Println("SDK Call onDestroy cb")

	sdk.Exit()
}

// GetGameObject2D implements IGameObject2D.
func (t TestPlayer) GetGameObject2D() *core.GameObject2D {
	return t.GameObject2D
}
